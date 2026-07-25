package proxy

import (
	"errors"
	"testing"
)

func TestLocateAndReplaceTopLevelModel(t *testing.T) {
	t.Parallel()

	input := []byte("{\n  \"input\": {\"model\": \"nested\"},\n  \"model\" : \"public-model\",\n  \"keep\": 1.0\n}")
	location, err := locateTopLevelModel(input)
	if err != nil {
		t.Fatalf("locateTopLevelModel() error = %v", err)
	}
	if location.value != "public-model" {
		t.Fatalf("model = %q", location.value)
	}
	output, err := replaceModel(input, location, "upstream-model")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"input\": {\"model\": \"nested\"},\n  \"model\" : \"upstream-model\",\n  \"keep\": 1.0\n}"
	if string(output) != want {
		t.Fatalf("output = %s\nwant = %s", output, want)
	}
}

func TestLocateTopLevelModelErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "missing", body: `{"input":"hello"}`, want: errMissingModel},
		{name: "non string", body: `{"model":42}`, want: errInvalidModel},
		{name: "duplicate", body: `{"model":"a","mo\u0064el":"b"}`, want: errDuplicateModel},
		{name: "array root", body: `["model"]`, want: errInvalidJSON},
		{name: "trailing", body: `{"model":"a"} true`, want: errInvalidJSON},
		{name: "invalid nested", body: `{"input":[1,],"model":"a"}`, want: errInvalidJSON},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := locateTopLevelModel([]byte(test.body))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
