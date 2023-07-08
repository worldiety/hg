package hg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type TplPreprocessor func(files []*TplFile) []*TplFile
type TplFilePreprocessor func(file *TplFile)

type htmlParser struct {
	tpls        *template.Template
	tplExecName string
	fsyss       []fs.FS
	funcs       template.FuncMap
	preProcs    []TplPreprocessor
}

type TplOption func(p *htmlParser)

func FS(fsys ...fs.FS) TplOption {
	for _, fsy := range fsys {
		matches, err := fs.Glob(fsy, "*.gohtml")
		if err != nil {
			panic(fmt.Errorf("invalid fs: %w", err))
		}

		if len(matches) == 0 {
			panic("empty fs")
		}
	}
	return func(p *htmlParser) {
		p.fsyss = append(p.fsyss, fsys...)
	}
}

func TplPreProc(f TplPreprocessor) TplOption {
	return func(p *htmlParser) {
		p.preProcs = append(p.preProcs, f)
	}
}

func TplPreProcEach(f TplFilePreprocessor) TplOption {
	return func(p *htmlParser) {
		p.preProcs = append(p.preProcs, func(files []*TplFile) []*TplFile {
			for _, file := range files {
				f(file)
			}
			return files
		})
	}
}

// Execute will invoke the given template Name instead of evaluating the entire template set anonymously.
func Execute(tplName string) TplOption {
	return func(p *htmlParser) {
		p.tplExecName = tplName
	}
}

func NamedFunc(name string, fun any) TplOption {
	return func(p *htmlParser) {
		p.funcs[name] = fun
	}
}

func MustParse[Model any](options ...TplOption) ViewFunc[Model] {
	f, err := Parse[Model](options...)
	if err != nil {
		panic(err)
	}

	return f
}

func Parse[Model any](options ...TplOption) (ViewFunc[Model], error) {
	parser := &htmlParser{
		tpls: template.New(""),
		funcs: template.FuncMap{
			// TODO still required?
			"toJSON": func(obj any) string {
				buf, err := json.Marshal(obj)
				if err != nil {
					panic(err)
				}
				return string(buf)
			},

			"map": func(args ...any) map[string]any {
				res := map[string]any{}
				for i := 0; i < len(args); i += 2 {
					res[args[i].(string)] = args[i+1]
				}

				return res
			},

			// unsafe func but required for writing custom inline "slot-definitions", see also str
			"html": func(args ...any) template.HTML {
				var sb strings.Builder
				for _, arg := range args {
					sb.WriteString(fmt.Sprintf("%v", arg))
				}

				return template.HTML(sb.String())
			},

			"str": func(args ...any) string {
				var sb strings.Builder
				for _, arg := range args {
					sb.WriteString(fmt.Sprintf("%v", arg))
				}

				return sb.String()
			},
		},
	}

	parser.funcs["evaluate"] = func(templateName string, data any) template.HTML {
		var tmp bytes.Buffer
		err := parser.tpls.ExecuteTemplate(&tmp, templateName, data)
		if err != nil {
			return template.HTML("<p>" + err.Error() + "</p>")
		}

		return template.HTML(tmp.String())
	}

	for _, option := range options {
		option(parser)
	}

	parser.tpls.Funcs(parser.funcs)
	files, err := loadAll(parser.fsyss)
	if err != nil {
		return nil, fmt.Errorf("cannot load files from filesystems. %w", err)
	}

	for _, proc := range parser.preProcs {
		files = proc(files)
	}

	for _, file := range files {
		subTpl := parser.tpls.New(file.Name)
		if _, err := subTpl.Parse(file.Data); err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", file.Name, err)
		}
	}

	return func(w http.ResponseWriter, r *http.Request, model Model) {
		if parser.tplExecName == "" {
			if err := parser.tpls.Execute(w, model); err != nil {
				slog.Error("cannot execute anonymous template", err)
				w.Write([]byte(fmt.Sprintf(`<p style="color:red">cannot parse anonymous template: %s</p>'`, err.Error())))
			}

			return
		}

		if err := parser.tpls.ExecuteTemplate(w, parser.tplExecName, model); err != nil {
			slog.Error("cannot execute named template", err)
			w.Write([]byte(fmt.Sprintf(`<p style="color:red">cannot parse '%s' template: %s</p>'`, parser.tplExecName, err.Error())))
		}
	}, nil

}

type TplFile struct {
	Name     string
	Data     string
	defines  []string
	requires []string
}

// RenameTplStmts replaces all {{ template "<oldname>" .}} occurrences with {{ template "<newname>" .}}
func (f *TplFile) RenameTplStmts(oldname, newname string) {
	oldname = `"` + oldname + `"`
	newname = `"` + newname + `"`
	f.Data = regexTemplate.ReplaceAllStringFunc(f.Data, func(s string) string {
		if regexIdentifer.FindString(s) == oldname {
			return regexIdentifer.ReplaceAllString(s, newname)
		}
		return s
	})
}

func (f *TplFile) require(tplName string) bool {
	return slices.Contains(f.requires, tplName)
}

func (f *TplFile) define(tplName string) bool {
	return slices.Contains(f.defines, tplName)
}

func loadAll(fssys []fs.FS) ([]*TplFile, error) {
	var res []*TplFile
	for _, fsys := range fssys {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if strings.HasSuffix(d.Name(), ".gohtml") {
				buf, err := fs.ReadFile(fsys, path)
				if err != nil {
					return fmt.Errorf("cannot read TplFile: %w", err)
				}

				if !utf8.Valid(buf) {
					return fmt.Errorf("no valid utf-8 bytes: %s", path)
				}
				res = append(res, &TplFile{
					Name: path,
					Data: string(buf),
				})
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

var regexDefine = regexp.MustCompile(`{{\s*define\s+"\w+"\s*}}`)
var regexTemplate = regexp.MustCompile(`{{\s*template\s+"\w+"\s*\.?\s*}}`)
var regexIdentifer = regexp.MustCompile(`"\w+"`)

func parseTemplateReferences(files []*TplFile) {
	for _, f := range files {
		defines := regexDefine.FindAllString(f.Data, -1)
		f.defines = append(f.defines, identsOf(defines)...)
		usages := regexTemplate.FindAllString(f.Data, -1)
		f.requires = append(f.requires, identsOf(usages)...)
	}
}

func identsOf(lines []string) []string {
	var res []string
	for _, line := range lines {
		if ident := regexIdentifer.FindString(line); ident != "" {
			ident, err := strconv.Unquote(ident)
			if err != nil {
				slog.Error("ignored invalid template identifier", slog.String("text", line))
				continue
			}

			res = append(res, ident)
		}
	}

	return res
}
