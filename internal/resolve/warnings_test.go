package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fairbearlab/rolodex/internal/model"
)

// truncatedCard is cut off mid-property, as an interrupted write or a full
// disk leaves it; the decoder reports it as malformed and skips it.
const truncatedCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Chen;Kath"

func appendRaw(t *testing.T, path, raw string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// A truncated merged.vcf lost its last card from final.vcf with no message:
// the cluster-id check cannot catch it, because the card is simply absent.
func TestRunRefusesMalformedMergedVCF(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	reviewPath := filepath.Join(dir, "review.vcf")
	mergedPath := filepath.Join(dir, "merged.vcf")
	outPath := filepath.Join(dir, "final.vcf")

	writeTestVCF(t, mergedPath, []model.MergedContact{{Contact: model.ParsedContact{FormattedName: "Keep Me"}}})
	appendRaw(t, mergedPath, truncatedCard)
	writeTestVCF(t, reviewPath, nil)
	writeTestReport(t, reportPath, model.Report{})

	err := Run(reportPath, reviewPath, mergedPath, outPath)
	if err == nil {
		t.Fatal("Run succeeded on a merged.vcf with a malformed entry")
	}
	for _, want := range []string{mergedPath, "1 malformed", "entry 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Errorf("final.vcf was written despite the refusal (stat: %v)", statErr)
	}
}

func TestLoadRefusesMalformedReviewVCF(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	reviewPath := filepath.Join(dir, "review.vcf")
	writeTestVCF(t, reviewPath, []model.MergedContact{{Contact: model.ParsedContact{FormattedName: "A"}}})
	appendRaw(t, reviewPath, truncatedCard)
	writeTestReport(t, reportPath, model.Report{})

	_, err := LoadReportAndReview(reportPath, reviewPath)
	if err == nil || !strings.Contains(err.Error(), reviewPath) || !strings.Contains(err.Error(), "1 malformed") {
		t.Fatalf("err = %v, want a refusal naming %s and its malformed entry", err, reviewPath)
	}
}
