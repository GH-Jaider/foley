package fake_test

import (
	"testing"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/enginetest"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

func TestFakeConformsBasic(t *testing.T) {
	enginetest.RunBasic(t, func(t *testing.T, opts vtengine.Options) vtengine.Engine {
		t.Helper()
		return fake.New(opts)
	})
}

func TestPuppetControls(t *testing.T) {
	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 10, Rows: 3, CellW: 8, CellH: 16}})
	defer func() { _ = e.Close() }()

	e.SetCell(0, 2, "→", vtengine.Style{Bold: true})
	e.AddPlacement(vtengine.Placement{ImageID: 7, Col: 1, Row: 1, PixelW: 16, PixelH: 16})
	e.SetImage(vtengine.ImageData{ID: 7, W: 2, H: 2, Generation: 1, Pix: make([]byte, 16)})

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got := f.RowText(2); got != "→" {
		t.Fatalf("RowText(2) = %q, want %q", got, "→")
	}
	if !f.CellAt(0, 2).Style.Bold {
		t.Fatal("style lost in snapshot")
	}
	if len(f.Graphics.Placements) != 1 || f.Graphics.Placements[0].ImageID != 7 {
		t.Fatalf("placements = %+v", f.Graphics.Placements)
	}
	img, err := e.ImagePixels(7)
	if err != nil || img.W != 2 {
		t.Fatalf("ImagePixels = %+v, %v", img, err)
	}
	if f.Graphics.Generation == 0 {
		t.Fatal("generation must advance on graphics mutations")
	}
}

func TestPlacementLayers(t *testing.T) {
	cases := []struct {
		z    int32
		want vtengine.Layer
	}{
		{z: -2000000000, want: vtengine.LayerBelowBG},
		{z: -1, want: vtengine.LayerBelowText},
		{z: 0, want: vtengine.LayerAboveText},
		{z: 5, want: vtengine.LayerAboveText},
	}
	for _, c := range cases {
		if got := (vtengine.Placement{Z: c.z}).Layer(); got != c.want {
			t.Fatalf("Layer(z=%d) = %d, want %d", c.z, got, c.want)
		}
	}
}
