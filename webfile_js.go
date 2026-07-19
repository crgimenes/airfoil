//go:build js

package main

import "syscall/js"

// onWeb marks the browser build. It is a constant so the compiler drops the
// branch that does not apply instead of carrying dead filesystem code.
const onWeb = true

// offerDownload hands bytes to the browser as a file download.
//
// A browser tab cannot write to a path, so this is what "save" means on the
// web: the file lands wherever the browser puts downloads, under the name given
// here. Browsers allow this without a prompt as long as the page has seen a
// real user interaction, which a click on the Save button satisfies.
func offerDownload(name string, data []byte) {
	buf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buf, data)

	parts := js.Global().Get("Array").New(1)
	parts.SetIndex(0, buf)
	blob := js.Global().Get("Blob").New(parts, map[string]any{"type": "application/octet-stream"})

	url := js.Global().Get("URL").Call("createObjectURL", blob)
	doc := js.Global().Get("document")
	a := doc.Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", name)
	doc.Get("body").Call("appendChild", a)
	a.Call("click")
	a.Call("remove")

	// Revoking the URL immediately can cancel a download that has not started
	// reading yet, so let it go stale on a timer instead.
	var release js.Func
	release = js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Get("URL").Call("revokeObjectURL", url)
		release.Release()
		return nil
	})
	js.Global().Call("setTimeout", release, 60000)
}
