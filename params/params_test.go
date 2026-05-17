package params

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseParams(t *testing.T) {

	if err := InitPresets(`{"sq":{"resize": true, "mode":"crop", "width":100,"height":100,"quality":90}}`); err != nil {
		t.Error("failed to parse presets")
	}

	var tests = []struct {
		input string
		path  string
		want  Params
	}{
		{
			"r1000x960,q89,w--h/example_path",
			"example_path",
			Params{
				Resize:     true,
				Width:      1000,
				Height:     960,
				Quality:    89,
				PixelRatio: 1,
				Watermarks: []Watermark{
					{Path: "h", Position: PositionSouthEast, Size: 100},
				},
			},
		},
		{
			"r250x141,c252x584x510x288,q85/foo%2Fbar",
			"foo/bar",
			Params{
				Resize:     true,
				Width:      250,
				Height:     141,
				Crop:       cropParams{X: 252, Y: 584, Width: 510, Height: 288},
				Quality:    85,
				PixelRatio: 1,
			},
		},
		{
			"/_sq/aBCD1aaaaaaaaaaaaaaa_b",
			"aBCD1aaaaaaaaaaaaaaa_b",
			Params{
				Resize:     true,
				Mode:       ModeCrop,
				Width:      100,
				Height:     100,
				Quality:    90,
				PixelRatio: 1,
			},
		},
		{
			"/_sq,p2/aBCD1aaaaaaaaaaaaaaa_b",
			"aBCD1aaaaaaaaaaaaaaa_b",
			Params{
				Resize:     true,
				Mode:       ModeCrop,
				Width:      100,
				Height:     100,
				Quality:    90,
				PixelRatio: 2,
			},
		},
		{
			"rc312x175,q45,p2/foobar",
			"foobar",
			Params{
				Resize:     true,
				Mode:       ModeCrop,
				Width:      312,
				Height:     175,
				Quality:    45,
				PixelRatio: 2,
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test%v", i), func(t *testing.T) {
			path, params, err := Parse(tt.input)
			if err != nil {
				t.Errorf("parsing failed for %v with %v", tt.input, err)
			}
			if path != tt.path {
				t.Errorf("got %s, want %s", tt.path, tt.path)
			}

			if !reflect.DeepEqual(params, tt.want) {
				t.Errorf("got %+v, want %+v", params, tt.want)
			}

		})
	}

}

func TestParseRejectsMalformedInputs(t *testing.T) {
	if err := InitPresets(`{"sq":{"resize": true, "mode":"crop", "width":100,"height":100,"quality":90}}`); err != nil {
		t.Fatal("failed to parse presets")
	}

	tests := []string{
		"rabc/foo",
		"r0x20/foo",
		"r-10x20/foo",
		"c1x2xax4/foo",
		"qabc/foo",
		"pabc/foo",
		"gf/foo",
		"wlogo-123/foo",
		"wlogo-12x34x56/foo",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Parse panicked for %q: %v", input, r)
				}
			}()

			if _, _, err := Parse(input); err == nil {
				t.Fatalf("expected an error for %q", input)
			}
		})
	}
}

func TestParseRejectsUnknownPreset(t *testing.T) {
	if err := InitPresets(`{"sq":{"resize": true, "mode":"crop", "width":100,"height":100,"quality":90}}`); err != nil {
		t.Fatal("failed to parse presets")
	}

	if _, _, err := Parse("_missing/foo"); err == nil {
		t.Fatal("expected unknown preset to return an error")
	}
}

func FuzzParseDoesNotPanic(f *testing.F) {
	seeds := []string{
		"",
		"/",
		"r100x100/foo",
		"rc312x175,q45,p2/foobar",
		"w--h/example_path",
		"wlogo-123/foo",
		"gf/foo",
		"%zz/foo",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Parse panicked for %q: %v", input, r)
			}
		}()

		_, _, _ = Parse(input)
	})
}
