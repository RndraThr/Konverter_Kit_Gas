package appreleases

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"konkit/internal/auth"
)

// zipAPK is an APK-shaped zip with native libraries for abis.
func zipAPK(t *testing.T, abis ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, name := range append([]string{"AndroidManifest.xml"}, func() []string {
		var libs []string
		for _, abi := range abis {
			libs = append(libs, "lib/"+abi+"/libapp.so")
		}
		return libs
	}()...) {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write([]byte(name))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestAPKABIFindsTheVariantPhoneType(t *testing.T) {
	if abi, err := apkABI(zipAPK(t, "armeabi-v7a")); err != nil || abi != "armeabi-v7a" {
		t.Fatalf("32-bit: %q %v", abi, err)
	}
	for name, data := range map[string][]byte{
		"64-bit":    zipAPK(t, "arm64-v8a"),
		"universal": zipAPK(t, "armeabi-v7a", "arm64-v8a", "x86_64"),
		"no code":   zipAPK(t),
	} {
		if _, err := apkABI(data); !errors.Is(err, ErrVariantABI) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestVariantABIPicksThePhoneType(t *testing.T) {
	for _, tc := range []struct {
		abis []string
		want string
	}{
		{nil, ""},
		{[]string{"arm64-v8a", "armeabi-v7a", "armeabi"}, ""},
		{[]string{"armeabi-v7a", "armeabi"}, "armeabi-v7a"},
		{[]string{"x86"}, "x86"},
	} {
		if got := variantABI(tc.abis); got != tc.want {
			t.Fatalf("%v: %q, want %q", tc.abis, got, tc.want)
		}
	}
}

func TestIntegrationThirtyTwoBitPhonesGetTheirOwnAPK(t *testing.T) {
	service, _ := integrationService(t)
	ctx := context.Background()
	actor := auth.Principal{}
	v2, err := service.Upload(ctx, actor, UploadInput{VersionName: "1.1.0", VersionCode: 2, Mandatory: true}, bytes.NewReader(zipAPK(t, "arm64-v8a")), auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddVariant(ctx, actor, v2.ID, bytes.NewReader(zipAPK(t, "arm64-v8a")), auth.ClientMeta{}); !errors.Is(err, ErrVariantABI) {
		t.Fatalf("64-bit as variant: %v", err)
	}
	v32 := zipAPK(t, "armeabi-v7a")
	variant, err := service.AddVariant(ctx, actor, v2.ID, bytes.NewReader(v32), auth.ClientMeta{})
	if err != nil || variant.ABI != "armeabi-v7a" || variant.SizeBytes != int64(len(v32)) {
		t.Fatalf("variant=%+v err=%v", variant, err)
	}
	// 3 has no 32-bit APK yet.
	if _, err := service.Upload(ctx, actor, UploadInput{VersionName: "1.2.0", VersionCode: 3}, bytes.NewReader(zipAPK(t, "arm64-v8a", "x")), auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	makePatches(t, service)

	// A 64-bit phone (or an app that does not say) gets the newest release.
	for _, abis := range [][]string{nil, {"arm64-v8a", "armeabi-v7a"}} {
		if update, err := service.LatestFor(ctx, "android", 1, abis); err != nil || update == nil || update.VersionCode != 3 {
			t.Fatalf("64-bit %v: %+v err=%v", abis, update, err)
		}
	}
	// A 32-bit phone gets 2, the newest with its APK, whole and mandatory.
	update, err := service.LatestFor(ctx, "android", 1, []string{"armeabi-v7a", "armeabi"})
	if err != nil || update == nil || update.VersionCode != 2 || !update.Mandatory || update.Patch != nil ||
		update.SizeBytes != variant.SizeBytes || update.Checksum != variant.Checksum || update.DownloadPath != "/api/v1/app/releases/"+v2.ID+"/apk?abi=armeabi-v7a" {
		t.Fatalf("32-bit: %+v err=%v", update, err)
	}
	if update, err := service.LatestFor(ctx, "android", 2, []string{"armeabi-v7a"}); err != nil || update != nil {
		t.Fatalf("32-bit on 2: %+v err=%v", update, err)
	}
	content, err := service.OpenVariant(ctx, v2.ID, "armeabi-v7a")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(content.Reader)
	_ = content.Reader.Close()
	if !bytes.Equal(data, v32) || content.Filename != "konkit-1.1.0-armeabi-v7a.apk" {
		t.Fatalf("variant content %d bytes %q", len(data), content.Filename)
	}
	if list, err := service.List(ctx); err != nil || len(list[1].Variants) != 1 || len(list[0].Variants) != 0 {
		t.Fatalf("list variants: %+v err=%v", list, err)
	}
	if err := service.Delete(ctx, actor, v2.ID, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenVariant(ctx, v2.ID, "armeabi-v7a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted variant: %v", err)
	}
}
