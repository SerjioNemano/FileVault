package web_test

import (
	"filevault/web"
	"testing"
	"testing/fstest"
)

func TestStatic(t *testing.T) {
	err := fstest.TestFS(web.Static(), "index.html", "app.js", "viewer.js", "style.css", "img/preview.png")
	if err != nil {
		t.Fatal(err)
	}
}
