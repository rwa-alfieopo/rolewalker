package web

import "embed"

//go:embed index.html app.js style.css image.png
var Assets embed.FS
