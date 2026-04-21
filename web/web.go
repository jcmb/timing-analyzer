package web

import _ "embed"

//go:embed index.html
var IndexHTML []byte

//go:embed index_server.html
var IndexServerHTML []byte

//go:embed chart.umd.min.js
var ChartJS []byte
