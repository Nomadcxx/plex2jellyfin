package jellywatch

import (
	"io"
	"strings"
	"testing"

	"github.com/Nomadcxx/jellywatch/embedded"
	"github.com/stretchr/testify/assert"
)

func TestHasFrontend(t *testing.T) {
	assert.True(t, embedded.HasFrontend(), "HasFrontend should return true when frontend is built")
}

func TestGetWebFSServesBuiltFrontend(t *testing.T) {
	webFS := GetWebFS()

	f, err := webFS.Open("index.html")
	if err != nil {
		t.Fatalf("open index.html: %v", err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	content := string(b)
	if strings.Contains(content, "<body>jellywatch</body>") {
		t.Fatalf("index.html is placeholder content, expected built frontend")
	}
	if !strings.Contains(content, "_next/static/") {
		t.Fatalf("index.html does not look like built Next.js output")
	}
}
