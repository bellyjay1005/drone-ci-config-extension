package plugin

import (
	"io/ioutil"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		file string
		err  bool
		mNil bool
		envs []string
		kind string
	}{
		{
			file: "testdata/.strithon.yml",
			err:  false,
			mNil: false,
			kind: "service",
		},
		{
			file: "testdata/.strithon-number.yml",
			err:  false,
			mNil: false,
			kind: "service",
		},
		{
			file: "testdata/.strithon-bad.yml",
			err:  true,
			mNil: true,
			kind: "",
		},
		{
			file: "testdata/.strithon-multiple.yml",
			err:  false,
			mNil: false,
			kind: "service",
		},
		{
			file: "testdata/.strithon-no-environ.yml",
			err:  false,
			mNil: false,
			envs: nil,
			kind: "service",
		},
	}
	for _, c := range cases {
		b, err := ioutil.ReadFile(c.file)
		source := string(b)
		if err != nil {
			t.Errorf("Error parsing test data file %v: %v", c.file, c.err)
		}
		m, errGot := ParsestrithonYml(source)
		if (errGot == nil) == c.err {
			t.Fatalf("Error mismatch, got %v, want err=nil to be %v", errGot, c.err)
		}
		if (m == nil) != c.mNil {
			t.Fatalf("Incorrect bellyjay1005 struct. Should be nil: %v", c.mNil)
		}
		if m != nil && m.Kind != c.kind {
			t.Fatalf("Incorrect kind, got %v want %v", m.Kind, c.kind)
		}
		if m != nil && c.envs != nil && m.Metadata.Environments[0].Account == "" {
			t.Fatalf("Account number not parsed")
		}
	}
}
