package writer

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
	"github.com/fairbearlab/rolodex/internal/parser"
)

// TestWritePhotoRoundTrips: the writer builds every content line itself, and
// PHOTO is the one property whose value is bytes, not text. It has to go out
// base64-encoded with ENCODING=b (and the TYPE the export carried) so that the
// parser — and Apple and Google — read the same bytes back. A photo that comes
// out wrong is silent data loss on the field a user notices first.
func TestWritePhotoRoundTrips(t *testing.T) {
	photo := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, ';', ':', '\\', '\n'}
	contacts := []model.MergedContact{
		{Contact: model.ParsedContact{
			FamilyName: "Lee", GivenName: "David", FormattedName: "David Lee",
			Photo: photo, PhotoType: "JPEG",
		}, Sources: []model.Source{model.SourceICloud}},
		{Contact: model.ParsedContact{
			FamilyName: "Baker", GivenName: "Bob", FormattedName: "Bob Baker",
			Photo: []byte("untyped"),
		}, Sources: []model.Source{model.SourceGoogle}},
	}

	var buf bytes.Buffer
	if err := Write(&buf, contacts); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"PHOTO;ENCODING=b;TYPE=JPEG:" + base64.StdEncoding.EncodeToString(photo),
		"PHOTO;ENCODING=b:" + base64.StdEncoding.EncodeToString([]byte("untyped")),
	} {
		if !strings.Contains(out, want+"\r\n") {
			t.Errorf("output lacks line %q:\n%s", want, out)
		}
	}

	again, warns, err := parser.Parse(strings.NewReader(out), model.SourceUnknown)
	if err != nil || len(warns) != 0 || len(again) != 2 {
		t.Fatalf("re-parse: err=%v warnings=%v contacts=%d", err, warns, len(again))
	}
	if !bytes.Equal(again[0].Photo, photo) || again[0].PhotoType != "JPEG" {
		t.Errorf("typed photo changed in the round trip: got %v (%q), want %v (JPEG)", again[0].Photo, again[0].PhotoType, photo)
	}
	if string(again[1].Photo) != "untyped" || again[1].PhotoType != "" {
		t.Errorf("untyped photo changed in the round trip: got %q (%q)", again[1].Photo, again[1].PhotoType)
	}
}
