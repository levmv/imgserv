package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestParseParams(t *testing.T) {

	if err := parsePresets(`{"sq":{"resize": true, "mode":"crop", "width":100,"height":100,"quality":90}}`); err != nil {
		t.Error("failed to parse presets")
	}

	var tests = []struct {
		input string
		path  string
		want  string
	}{
		{
			"r1000x960,q89,w--h/example_path",
			"example_path",
			`{"resize":true,"mode":"","width":1000,"height":960,"crop":{"x":0,"y":0,"width":0,"height":0},"quality":89,"watermarks":[{"path":"h","position":"se","size":100}]}`,
		},
		{
			"r250x141,c252x584x510x288,q85/foo%2Fbar",
			"foo/bar",
			`{"resize":true,"mode":"","width":250,"height":141,"crop":{"x":252,"y":584,"width":510,"height":288},"quality":85}`,
		},
		{
			"/_sq/aBCD1aaaaaaaaaaaaaaa_b",
			"aBCD1aaaaaaaaaaaaaaa_b",
			`{"resize":true,"mode":"crop","width":100,"height":100,"crop":{"x":0,"y":0,"width":0,"height":0},"quality":90}`,
		},
		{
			"/_sq,p2/aBCD1aaaaaaaaaaaaaaa_b",
			"aBCD1aaaaaaaaaaaaaaa_b",
			`{"resize":true,"mode":"crop","width":100,"height":100,"crop":{"x":0,"y":0,"width":0,"height":0},"quality":90,"pixel_ratio":2}`,
		},
		{
			"rc312x175,q45,p2/foobar",
			"foobar",
			`{"resize":true,"mode":"crop","width":312,"height":175,"crop":{"x":0,"y":0,"width":0,"height":0},"quality":45,"pixel_ratio":2}`,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test%v", i), func(t *testing.T) {
			path, params, err := parseParams(tt.input)
			if err != nil {
				t.Errorf("parsing failed for %v with %v", tt.input, err)
			}
			if path != tt.path {
				t.Errorf("got %s, want %s", tt.path, tt.path)
			}

			js, _ := json.Marshal(params)
			if string(js) != tt.want {
				t.Errorf("got %s, want %s", js, tt.want)
			}

		})
	}

}
