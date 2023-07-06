package hg

import "embed"

// Assets contains a small js shim containing the request-response utilities and the idiomorph library.
//
//go:embed assets/hg.js assets/idiomorph.js
var Assets embed.FS

// Tailwind contains a bundled CDN version of tailwind, which uses a JIT compiler to support all tailwind features
// on the fly. Its usage is not really recommended for production sites, however it works good enough for small things.
// Also, you can always decide to not include it and instead use the tailwind cli to compile a tailored version.
//
//go:embed assets/tailwind.js
var Tailwind embed.FS
