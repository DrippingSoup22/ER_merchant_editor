module github.com/DrippingSoup22/ER_merchant_editor

// Floor set by the GUI deps (Gio v0.10.x needs >=1.24, x/image v0.44 >=1.25).
// The apt toolchain is go1.22.2 with GOTOOLCHAIN=auto, which fetches the
// required toolchain once and caches it.
go 1.25.0

require (
	gioui.org v0.10.1
	gioui.org/x v0.10.1
	github.com/klauspost/compress v1.17.11
	github.com/ncruces/zenity v0.10.14
	golang.org/x/image v0.44.0
)

require (
	gioui.org/shader v1.0.8 // indirect
	github.com/akavel/rsrc v0.10.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/dchest/jsmin v0.0.0-20220218165748-59f39799265f // indirect
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/josephspurrier/goversioninfo v1.4.1 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/randall77/makefat v0.0.0-20210315173500-7ddd0e42c844 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tc-hib/go-winres v0.3.3 // indirect
	github.com/tc-hib/winres v0.2.1 // indirect
	github.com/urfave/cli/v2 v2.25.7 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/exp/shiny v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

tool github.com/tc-hib/go-winres
