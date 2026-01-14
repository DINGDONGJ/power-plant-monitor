//go:build ignore

// 这个文件说明如何生成 resource_windows.syso
//
// 方法一：使用 rsrc 工具（推荐）
//   go install github.com/akavel/rsrc@latest
//   rsrc -manifest monitor-web.exe.manifest -o resource_windows.syso
//
// 方法二：使用 windres（需要 MinGW）
//   windres -i resource.rc -o resource_windows.syso -O coff
//
// 生成后，go build 会自动链接 .syso 文件

package main
