package tilepix

import (
	"fmt"
	"image/color"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/pixelgl"

	log "github.com/sirupsen/logrus"
)

/*
  __  __
 |  \/  |__ _ _ __
 | |\/| / _` | '_ \
 |_|  |_\__,_| .__/
             |_|
*/

// Map is a TMX file structure representing the map as a whole.
type Map struct {
	Version     string `xml:"title,attr"`
	Orientation string `xml:"orientation,attr"`
	// Width is the number of tiles - not the width in pixels
	Width int `xml:"width,attr"`
	// Height is the number of tiles - not the height in pixels
	Height       int            `xml:"height,attr"`
	TileWidth    int            `xml:"tilewidth,attr"`
	TileHeight   int            `xml:"tileheight,attr"`
	Properties   []*Property    `xml:"properties>property"`
	Tilesets     []*Tileset     `xml:"tileset"`
	Layers       []*Layer       `xml:"layer"`
	ObjectGroups []*ObjectGroup `xml:"objectgroup"`
	Infinite     bool           `xml:"infinite,attr"`
	ImageLayers  []*ImageLayer  `xml:"imagelayer"`

	canvas *pixelgl.Canvas
}

// DrawAll will draw all tile layers and image layers to the target.
// Tile layers are first draw to their own `pixel.Batch`s for efficiency.
// All layers are drawn to a `pixel.Canvas` before being drawn to the target.
//
// - target - The target to draw layers to.
// - clearColour - The colour to clear the maps' canvas before drawing.
// - mat - The matrix to draw the canvas to the target with.
func (m *Map) DrawAll(target pixel.Target, clearColour color.Color, mat pixel.Matrix) error {
	if m.canvas == nil {
		m.canvas = pixelgl.NewCanvas(m.bounds())
	}
	m.canvas.Clear(clearColour)

	for _, l := range m.Layers {
		if err := l.Draw(m.canvas); err != nil {
			log.WithError(err).Error("Map.DrawAll: could not draw layer")
			return err
		}
	}

	for _, il := range m.ImageLayers {
		// The matrix shift is because images are drawn from the top-left in Tiled.
		if err := il.Draw(m.canvas, pixel.IM.Moved(pixel.V(0, m.pixelHeight()))); err != nil {
			log.WithError(err).Error("Map.DrawAll: could not draw image layer")
			return err
		}
	}

	m.canvas.Draw(target, mat.Moved(m.bounds().Center()))

	return nil
}

// GetImageLayerByName returns a Map's ImageLayer by its name
func (m *Map) GetImageLayerByName(name string) *ImageLayer {
	for _, l := range m.ImageLayers {
		if l.Name == name {
			return l
		}
	}
	return nil
}

// GetLayerByName returns a Map's Layer by its name
func (m *Map) GetLayerByName(name string) *Layer {
	for _, l := range m.Layers {
		if l.Name == name {
			return l
		}
	}
	return nil
}

// GetObjectLayerByName returns a Map's ObjectGroup by its name
func (m *Map) GetObjectLayerByName(name string) *ObjectGroup {
	for _, l := range m.ObjectGroups {
		if l.Name == name {
			return l
		}
	}
	return nil
}

func (m *Map) String() string {
	return fmt.Sprintf(
		"Map{Version: %s, Tile dimensions: %dx%d, Properties: %v, Tilesets: %v, Layers: %v, Object layers: %v, Image layers: %v}",
		m.Version,
		m.Width,
		m.Height,
		m.Properties,
		m.Tilesets,
		m.Layers,
		m.ObjectGroups,
		m.ImageLayers,
	)
}

// bounds will return a pixel rectangle representing the width-height in pixels.
func (m *Map) bounds() pixel.Rect {
	return pixel.R(0, 0, m.pixelWidth(), m.pixelHeight())
}

func (m *Map) pixelWidth() float64 {
	return float64(m.Width * m.TileWidth)
}
func (m *Map) pixelHeight() float64 {
	return float64(m.Height * m.TileHeight)
}

func (m *Map) decodeGID(gid GID) (*DecodedTile, error) {
	if gid == 0 {
		return NilTile, nil
	}

	gidBare := gid &^ gidFlip

	for i := len(m.Tilesets) - 1; i >= 0; i-- {
		if m.Tilesets[i].FirstGID <= gidBare {
			return &DecodedTile{
				ID:             ID(gidBare - m.Tilesets[i].FirstGID),
				Tileset:        m.Tilesets[i],
				HorizontalFlip: gid&gidHorizontalFlip != 0,
				VerticalFlip:   gid&gidVerticalFlip != 0,
				DiagonalFlip:   gid&gidDiagonalFlip != 0,
				Nil:            false,
			}, nil
		}
	}

	log.WithError(ErrInvalidGID).Error("Map.decodeGID: GID is invalid")
	return nil, ErrInvalidGID
}

func (m *Map) decodeLayers() error {
	// Decode tile layers
	for _, l := range m.Layers {
		gids, err := l.decode(m.Width, m.Height)
		if err != nil {
			log.WithError(err).Error("Map.decodeLayers: could not decode layer")
			return err
		}

		l.DecodedTiles = make([]*DecodedTile, len(gids))
		for j := 0; j < len(gids); j++ {
			decTile, err := m.decodeGID(gids[j])
			if err != nil {
				log.WithError(err).Error("Map.decodeLayers: could not GID")
				return err
			}
			l.DecodedTiles[j] = decTile
		}
	}

	// Decode object layers
	for _, og := range m.ObjectGroups {
		if err := og.decode(); err != nil {
			log.WithError(err).Error("Map.decodeLayers: could not decode Object Group")
			return err
		}
	}

	return nil
}

func (m *Map) setParents() {
	for _, p := range m.Properties {
		p.setParent(m)
	}
	for _, t := range m.Tilesets {
		t.setParent(m)
	}
	for _, og := range m.ObjectGroups {
		og.setParent(m)
	}
	for _, im := range m.ImageLayers {
		im.setParent(m)
	}
	for _, l := range m.Layers {
		l.setParent(m)
	}
}