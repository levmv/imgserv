package params

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/levmv/imgserv/vips"
)

type cropParams struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Watermark struct {
	Path      string       `json:"path"`
	Position  PositionType `json:"position"`
	PositionX int          `json:"position_x"`
	PositionY int          `json:"position_y"`
	Size      int          `json:"size"`
}

const ModeContain string = "contain"
const ModeFill string = "fill"
const ModeCrop string = "crop"

type GravityType string

const (
	GravityNone   GravityType = ""
	GravityCenter GravityType = "center"
	GravitySmart  GravityType = "smart"
)

type PositionType string

const (
	PositionCoords    PositionType = ""
	PositionNorth     PositionType = "n"
	PositionNorthEast PositionType = "ne"
	PositionEast      PositionType = "e"
	PositionSouthEast PositionType = "se"
	PositionSouth     PositionType = "s"
	PositionSouthWest PositionType = "sw"
	PositionWest      PositionType = "w"
	PositionNorthWest PositionType = "nw"
	PositionCenter    PositionType = "c"
)

type Params struct {
	Resize     bool        `json:"resize"`
	Mode       string      `json:"mode"`
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Upscale    bool        `json:"upscale,omitempty"`
	Crop       cropParams  `json:"crop,omitempty"`
	Gravity    GravityType `json:"gravity,omitempty"`
	Quality    int         `json:"quality"`
	PixelRatio float64     `json:"pixel_ratio,omitempty"`
	Watermarks []Watermark `json:"watermarks,omitempty"`
}

func defaultParams() Params {
	params := Params{}
	params.Quality = 80
	params.Upscale = false
	params.Gravity = GravityNone
	params.PixelRatio = 1
	return params
}

var presets map[string]Params

// InitPresets parse presets json config and save info to use when parsing urls
func InitPresets(strPresets string) error {
	var tmp map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strPresets), &tmp); err != nil {
		return fmt.Errorf("can't parse presets (%w)", err)
	}
	presets = make(map[string]Params)
	for name, preset := range tmp {
		p := defaultParams()
		if err := json.Unmarshal([]byte(preset), &p); err != nil {
			return err
		}
		presets[name] = p
	}
	return nil
}

func Parse(inputQuery string) (string, Params, error) {

	params := defaultParams()
	var exist bool

	paramsPart, path, found := strings.Cut(strings.Trim(inputQuery, "/"), "/")
	if !found {
		return "", params, fmt.Errorf("incorrect inputQuery %s", inputQuery)
	}

	path, err := url.QueryUnescape(path)
	if err != nil {
		return "", params, errors.New("incorrect escaping of path")
	}

	for _, s := range strings.Split(paramsPart, ",") {

		if len(s) < 2 {
			return path, params, errors.New("incorrect input")
		}

		name := s[:1]
		value := s[1:]

		switch name {
		case "r":
			if value[:1] == "f" {
				params.Mode = ModeFill
				value = value[1:]
			} else if value[:1] == "c" {
				params.Mode = ModeCrop
				value = value[1:]
			}
			sizes := strings.Split(value, "x")
			if len(sizes) == 2 {
				params.Resize = true
				params.Width, _ = strconv.Atoi(sizes[0])
				params.Height, _ = strconv.Atoi(sizes[1])
			} else if len(sizes) == 1 && sizes[0] != "" {
				params.Resize = true
				params.Width, _ = strconv.Atoi(sizes[0])
			} else {
				return path, params, errors.New("wrong resize values count")
			}
		case "c":
			numbers := strings.Split(value, "x")
			if len(numbers) != 4 {
				return path, params, errors.New("wrong crop values count")
			}
			cropParams := cropParams{}
			cropParams.X, _ = strconv.Atoi(numbers[0])
			cropParams.Y, _ = strconv.Atoi(numbers[1])
			cropParams.Width, _ = strconv.Atoi(numbers[2])
			cropParams.Height, _ = strconv.Atoi(numbers[3])
			params.Crop = cropParams
		case "q":
			params.Quality, _ = strconv.Atoi(value)

		case "g":
			subName := value[:1]
			if subName == "f" {
				panic("not implemented")
			}
			if subName == "s" {
				params.Gravity = GravitySmart
			}

		case "w":
			if value == "--h" { // legacy support
				params.Watermarks = append(params.Watermarks, Watermark{
					Path:     "h",
					Position: PositionSouthEast,
					Size:     100,
				})
				break
			}
			wm := Watermark{
				Position: PositionSouthEast,
				Size:     100,
			}

			opts := strings.Split(value, "-")
			if len(opts) > 2 {
				wm.Size, _ = strconv.Atoi(opts[2])
			}
			if len(opts) > 1 {
				if len(opts[1]) > 2 {
					wm.Position = PositionCoords
					coords := strings.Split(opts[1], "x")
					wm.PositionX, err = strconv.Atoi(coords[0])
					if err != nil {
						return path, params, errors.New("wrong watermark coordinate")
					}
					wm.PositionY, err = strconv.Atoi(coords[1])
					if err != nil {
						return path, params, errors.New("wrong watermark coordinate")
					}
				} else {
					wm.Position = PositionType(opts[1])
				}
			}
			wm.Path, _ = url.QueryUnescape(opts[0])
			params.Watermarks = append(params.Watermarks, wm)
		case "p":
			params.PixelRatio, _ = strconv.ParseFloat(value, 64)
		case "_":
			params, exist = presets[value]
			if !exist {
				return path, params, errors.New("unknown preset " + value)
			}
		case "n":
		default:
			return path, params, errors.New("unsupported param " + name)
		}
	}
	return path, params, nil
}

func Gravity2Vips(str GravityType) vips.Interesting {
	switch str {
	case GravityCenter:
		return vips.InterestingCentre
	case GravitySmart:
		return vips.InterestingAttention
	default:
		return vips.InterestingNone
	}
}
