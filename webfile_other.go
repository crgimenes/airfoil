//go:build !js

package main

// onWeb marks the browser build; see webfile_js.go. Off the browser the app
// owns real files, so the download path is never taken.
const onWeb = false

// offerDownload only ever runs in the browser build.
func offerDownload(_ string, _ []byte) {}
