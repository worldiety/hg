# hg
hg is a small **h**tml library for **g**o, which provides some comforting utilities and encourages an event based html SSR (server-side rendering) flow style.
It is compatible with the standard library and can be integrated into any router and http server setup.

## Influenced by

This library has been influenced by the following patterns, theories and technologies:
* The [elm](https://elm-lang.org/) programming language and its state and message system (see `hg.Update`)
* The [Vue component and slots](https://vuejs.org/guide/components/slots.html#slot-content-and-outlet) mechanism (see the template helper functions `map`, `html`, `str` and `evaluate`)
* The [Vue hydration](https://vuejs.org/guide/scaling-up/ssr.html) concept (see [Idiomorph](https://github.com/bigskysoftware/idiomorph))
* The [Vue HTML5 history mode](https://router.vuejs.org/guide/essentials/history-mode.html#html5-mode) (see the js-helper library)
* The [Vue hot reload](https://vue-loader.vuejs.org/guide/hot-reload.html) (see the hotReload js helper function and [reflex](https://github.com/cespare/reflex) or an [IntelliJ-Plugin](https://youtrack.jetbrains.com/issue/GO-11119#focus=Comments-27-4901631.0-0))
* Spring Boot [Thymeleaf Redirect](https://www.baeldung.com/spring-redirect-and-forward) (see `hg.Redirect`)
* [Blazor](https://learn.microsoft.com/de-de/aspnet/core/blazor/?view=aspnetcore-7.0) (but hg is cheaper and stateless. Instead of allocating server resources and bind them to permanent websocket connections, the page state is offloaded and embedded into the delivered page)

## Conceptual data flow

![flow](flow.svg)

## minimal example

greeting.go
```go
package main

import (
	"fmt"
	"github.com/worldiety/hg"
	"github.com/worldiety/hg-example/internal/helloworld"
	"github.com/worldiety/hg-example/internal/helloworld/web"
	"net/http"
)

type PageState struct {
	Title  string
	Greets string
}

func main() {
	fmt.Println("starting")
	page := hg.Handler(
		hg.MustParse[PageState](
			hg.FS(web.Templates),
			hg.Execute("index"),
			hg.NamedFunc("page", func() string { return "greeting" }),
		),
		hg.OnRequest(
			func(r *http.Request, model PageState) PageState {
				model.Title = "greetings from hg"
				model.Greets = helloworld.SayHello("Torben")
				return model
			},
		),
	)
	http.ListenAndServe("localhost:8080", page)
}
```

web.go
```go
package web

import "embed"

//go:embed  pages/*.gohtml index/*.gohtml
var Templates embed.FS

```

index.gohtml
```html
{{define "index"}}
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta id="_state" content='{{toJSON .}}'>
        <meta charset="UTF-8">
        <meta name="viewport"
              content="width=device-width, user-scalable=no, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0">
        <title>{{.Title}}</title>
        <script src="/assets/tailwind.js"></script>
        <script src="/assets/idiomorph.js"></script>

        <script src="/assets/gohtml.js" type="text/javascript"></script>
    </head>


    
    <body class="h-screen w-screen absolute bg-gray-200 dark:bg-gray-900">
    {{evaluate page .}}
    </body>
    </html>
{{end}}
```

greeting.gohtml

```html
{{define "greeting"}}
    <p>Just saying {{.Greets}}</p>
{{end}}

```

## Why?

At first, encouraging SSR in 2023 may seem to be out of time, however there is the interesting trend of all large SPA frameworks, to provide SSR concepts anyway, where the page is pre-rendered and delivered and hydrated at the client side, to get the best of both worlds.
On the other hand, most SPA applications are just pure technical overkill, because not a single unique advantage is used.
Take a look of this excerpt of possible features:

* Offline and Caching Support
* Complex interactive and responsive UI like Editors
* faster Response-Time, due to smaller requests (usually REST)
* Offloading Rendering to the Client
* enforces some kind of frontend-backend architecture (typically REST or graphql as a communication protocol)
* Optimizations only work effectively with a lot of handcrafted work, otherwise the result will usually contain everything at once
* complex and awesome UX

And, there are also a lot of disadvantages:

* SPA frameworks are generally incompatible across regular major version updates, which becomes really expensive over time
* SPA frameworks require an often used supply chain attack vector to be buildable at all, e.g. npm. There are regularly massive npm security flaws and compromised third party libraries, which are used without any actually benefit (left pad or is even/is odd anyone?)
* Today, search-engines support the execution of Javascript, but it is generally not compatible with the asynchronous workflow of SPA frameworks.
* Without transpilers and the usage of other languages like Typescript, maintaining larger projects is infeasible.
* Even though there have been millions of dollars and person hours already spend, Javascript has an unprofessional background without the possibility to detach from its past.
This still results in a lot of intransparent optimizations, compatibility shims and re-inventions to apply any sort of workarounds.
For example tree shaking (respective dead code elimination) and other compiler or link time optimizations are solved problems, which are well known and have been implemented more than 50 years ago for any reasonable language.
* You can optimize it with a lot of effort, but the page must first load (more or less) everything anyway.
* You have to create a REST API (or similar) without any actual need.
This means more work to implement and test, so its more expensive.
Also, creating meaningful APIs which are not just some repository CRUDs are not that easy.

Here we are.
Even though proposing to go back to SSR looks like a step backwards and may even cause the absence of a defined (RPC-)API, it looks like we can be optimistic regarding maintainability, when considering improvements in our understanding of software architecture patterns.
Our assumptions are (still to be proofed):

* faster time to market
* less expensive to develop
* less expensive to maintain
* more backend developers can be fullstack developers
* more than good enough for low-frequently used tools, especially for domain experts
* easier to debug and profile
* reliable and state-of-the-art compilers and toolings
* better architecture than the average mainstream SPA framework, especially when comparing with the typical CRUD API styles.
* no additional harm for the software architecture in general, due to layered architectures and the application of domain-driven design patterns.

