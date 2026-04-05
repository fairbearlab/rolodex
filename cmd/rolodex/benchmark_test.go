package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func generateVCF(path string, count int, prefix string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := 0; i < count; i++ {
		fmt.Fprintf(f, `BEGIN:VCARD
VERSION:3.0
N:Family%d;%sGiven%d;;;
FN:%sGiven%d Family%d
EMAIL;TYPE=HOME:%s.user%d@example.com
TEL;TYPE=CELL:555-%04d-%04d
ORG:Company%d
END:VCARD
`, i, prefix, i, prefix, i, i, prefix, i, i%10000, i, i%100)
	}
	return nil
}

func BenchmarkMerge1000(b *testing.B) {
	tmpDir := b.TempDir()
	icloudPath := filepath.Join(tmpDir, "icloud.vcf")
	googlePath := filepath.Join(tmpDir, "google.vcf")

	// Generate 500 contacts per source with ~10% overlap
	if err := generateVCF(icloudPath, 500, "ic"); err != nil {
		b.Fatal(err)
	}

	// Google contacts: 450 unique + 50 that share emails with iCloud
	f, err := os.Create(googlePath)
	if err != nil {
		b.Fatal(err)
	}
	// 50 overlapping (same email as iCloud contacts 0-49)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(f, `BEGIN:VCARD
VERSION:3.0
N:Family%d;GGiven%d;;;
FN:GGiven%d Family%d
EMAIL;TYPE=HOME:ic.user%d@example.com
TEL;TYPE=CELL:555-%04d-%04d
ORG:Company%d
END:VCARD
`, i, i, i, i, i, i+5000, i, i%100)
	}
	// 450 unique
	for i := 50; i < 500; i++ {
		fmt.Fprintf(f, `BEGIN:VCARD
VERSION:3.0
N:GFamily%d;GGiven%d;;;
FN:GGiven%d GFamily%d
EMAIL;TYPE=HOME:g.user%d@example.com
TEL;TYPE=CELL:555-%04d-%04d
ORG:GCompany%d
END:VCARD
`, i, i, i, i, i, i+5000, i, i%100)
	}
	f.Close()

	outPath := filepath.Join(tmpDir, "merged.vcf")
	reviewPath := filepath.Join(tmpDir, "review.vcf")
	reportPath := filepath.Join(tmpDir, "report.json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := merge(icloudPath, googlePath, outPath, reviewPath, reportPath); err != nil {
			b.Fatal(err)
		}
	}
}
