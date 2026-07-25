package i18n

import "testing"

func TestLanguageFromLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		locale string
		want   Language
	}{
		{locale: "zh_CN.UTF-8", want: SimplifiedChinese},
		{locale: "zh-Hant", want: SimplifiedChinese},
		{locale: "en_US.UTF-8", want: English},
		{locale: "", want: English},
	}
	for _, test := range tests {
		if got := languageFromLocale(test.locale); got != test.want {
			t.Errorf("languageFromLocale(%q) = %q, want %q", test.locale, got, test.want)
		}
	}
}

func TestCatalogFallback(t *testing.T) {
	t.Parallel()

	chinese := New(SimplifiedChinese)
	if got := chinese.Message("unknown_model"); got != "请求的模型尚未配置。" {
		t.Fatalf("Chinese message = %q", got)
	}
	if got := chinese.Message("missing-key"); got != "missing-key" {
		t.Fatalf("missing key fallback = %q", got)
	}
}
