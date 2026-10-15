package template

import _ "embed"

// PPTX is the master Laporan Akhir PowerPoint template, embedded so the
// server binary does not depend on source files at runtime.
//
//go:embed final_report_template.pptx
var PPTX []byte
