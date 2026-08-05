package render

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"sort"
	"time"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

//go:embed Go-Regular.ttf
var goFont []byte

// Layout constants. Width is fixed; height grows with the number of currencies.
const (
	imgWidth = 1280
	padding  = 24

	// Font sizes (in points).
	titleSize    = 28
	subtitleSize = 16
	headerSize   = 20
	rowSize      = 22
)

// rowMul scales font size to row height. Variable (not const) so the int cast
// works on the expression.
var rowMul = 1.6

// Palette tuned for readability on Discord's dark theme.
var (
	bgColor     = color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff}
	headerColor = color.RGBA{R: 0x2a, G: 0x2a, B: 0x3a, A: 0xff}
	titleColor  = color.RGBA{R: 0xf1, G: 0xc4, B: 0x0f, A: 0xff}
	subColor    = color.RGBA{R: 0xb0, G: 0xb0, B: 0xc0, A: 0xff}
	nameColor   = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	curColor    = color.RGBA{R: 0x90, G: 0x90, B: 0xa0, A: 0xff}
	buyColor    = color.RGBA{R: 0x4a, G: 0xc8, B: 0x6e, A: 0xff}
	sellColor   = color.RGBA{R: 0xe6, G: 0x6a, B: 0x6a, A: 0xff}
	bulkColor   = color.RGBA{R: 0xf1, G: 0xc4, B: 0x0f, A: 0xff}
	altRowColor = color.RGBA{R: 0x25, G: 0x25, B: 0x35, A: 0xff}
	textColor   = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	ruleColor   = color.RGBA{R: 0x40, G: 0x40, B: 0x55, A: 0xff}
)

// Recommendation is the shape render.Table reads. Fields mirror the analysis
// recommendation so the caller can convert directly.
type Recommendation struct {
	Name       string
	Display    string
	Current    float64
	BuyTarget  float64
	SellTarget float64
	BulkSell   float64
	HalfLife   time.Duration
}

// Table renders a currency analysis table to an image. The image height grows
// with the number of currencies so it always fits the data.
func Table(recs []Recommendation, league string, title string) image.Image {
	sorted := make([]Recommendation, len(recs))
	copy(sorted, recs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return spreadOf(sorted[i]) > spreadOf(sorted[j])
	})

	face, err := loadFace(rowSize)
	if err != nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	headerFace, _ := loadFace(headerSize)
	titleFace, _ := loadFace(titleSize)
	subFace, _ := loadFace(subtitleSize)

	rowH := int(float64(rowSize) * rowMul)
	// Header is two lines (label + small hint). The label sits at headerSize
	// and the hint drops below by subtitleSize, with padding.
	headerH := int(float64(headerSize)*rowMul) + int(float64(subtitleSize)*rowMul) + 6
	titleH := int(float64(titleSize) * rowMul)
	footH := int(float64(rowSize) * rowMul)

	height := padding*2 + titleH + rowH + headerH + rowH*len(sorted) + footH
	dc := gg.NewContext(imgWidth, height)
	dc.SetColor(bgColor)
	dc.DrawRectangle(0, 0, float64(imgWidth), float64(height))
	dc.Fill()

	// Title
	dc.SetFontFace(titleFace)
	dc.SetColor(titleColor)
	drawText(dc, title, padding, padding+titleSize)

	// Subtitle
	dc.SetFontFace(subFace)
	dc.SetColor(subColor)
	drawText(dc, "League: "+league+"  |  All prices in Exalted Orbs  |  Bulk = order \u2265 500", padding, padding+titleH+8)
	// Header row
	y := padding + titleH + rowH
	dc.SetColor(headerColor)
	dc.DrawRectangle(0, float64(y), float64(imgWidth), float64(headerH))
	dc.Fill()
	dc.SetColor(textColor)
	dc.SetFontFace(headerFace)
	cols := columnPositions()
	// Each header is (label, hint). Empty hint means render the label only.
	// Hints are deliberately short: each numeric column is only ~130px wide,
	// so the hint must fit inside one column or it visually bleeds into the
	// next one (the right-edge of hint N sits right where the left of hint N+1
	// begins).
	headers := []struct{ label, hint string }{
		{"Currency", "what you're trading"},
		{"Current", "live now"},
		{"Buy", "post here"},
		{"Sell", "post here"},
		{"Bulk", "stack of 500+"},
		{"Reversion", "expected hold time"},
		{"Spread", "buy→sell %"},
	}
	hintFace, _ := loadFace(13)
	// Currency (col 0) is left-aligned; all numeric columns are right-aligned
	// to the same right edge as the data. Hints sit *inside* the same column
	// and use a smaller font so they don't visually bleed into the next column.
	for i, h := range headers {
		if i == 0 {
			drawText(dc, h.label, cols[i].LeftX, y+headerSize)
		} else {
			drawTextRight(dc, h.label, cols[i].RightX, y+headerSize)
		}
		if h.hint != "" {
			dc.SetColor(subColor)
			dc.SetFontFace(hintFace)
			if i == 0 {
				drawText(dc, h.hint, cols[i].LeftX, y+headerSize+subtitleSize)
			} else {
				// Hint stays clear of the next column's data by leaving its
				// right edge a bit inside the column's own width.
				drawTextRight(dc, h.hint, cols[i].RightX-4, y+headerSize+subtitleSize)
			}
			dc.SetColor(textColor)
			dc.SetFontFace(headerFace)
		}
	}

	// Hairline under header
	y += headerH
	dc.SetColor(ruleColor)
	dc.DrawRectangle(0, float64(y), float64(imgWidth), 1)
	dc.Fill()

	// Currency rows
	dc.SetFontFace(face)
	for i, r := range sorted {
		if i%2 == 1 {
			dc.SetColor(altRowColor)
			dc.DrawRectangle(0, float64(y), float64(imgWidth), float64(rowH))
			dc.Fill()
		}
		// Name (left-aligned)
		dc.SetColor(nameColor)
		drawText(dc, displayOf(r), cols[0].LeftX, y+rowSize+6)
		// Current
		dc.SetColor(curColor)
		drawTextRight(dc, fmtFloat(r.Current), cols[1].RightX, y+rowSize+6)
		// Buy
		dc.SetColor(buyColor)
		drawTextRight(dc, fmtFloat(r.BuyTarget), cols[2].RightX, y+rowSize+6)
		// Sell
		dc.SetColor(sellColor)
		drawTextRight(dc, fmtFloat(r.SellTarget), cols[3].RightX, y+rowSize+6)
		// Bulk
		dc.SetColor(bulkColor)
		if r.BulkSell > r.SellTarget {
			drawTextRight(dc, fmtFloat(r.BulkSell), cols[4].RightX, y+rowSize+6)
		} else {
			drawTextRight(dc, "-", cols[4].RightX, y+rowSize+6)
		}
		// Reversion (OU half-life)
		dc.SetColor(subColor)
		drawTextRight(dc, fmtHalfLife(r.HalfLife), cols[5].RightX, y+rowSize+6)
		// Spread
		dc.SetColor(textColor)
		drawTextRight(dc, fmtSpread(spreadOf(r)), cols[6].RightX, y+rowSize+6)

		y += rowH
	}

	// Footer
	dc.SetColor(subColor)
	drawText(dc, "Generated by kingsmarch-koinery", padding, y+rowSize+4)

	return dc.Image()
}

// column defines the left and right x of one column. Text is either left-
// aligned (at LeftX) or right-aligned (at RightX), depending on whether the
// content is a name or a number.
type column struct {
	LeftX  int
	RightX int
}

func columnPositions() [7]column {
	// Layout (left-to-right):
	//
	//   [Name L]  |  [Current R] [Buy R] [Sell R] [Bulk R] [Reversion R] [Spread R]  |  right margin
	//
	// The right margin must be wide enough that the "Spread" column has air
	// around it after Discord scales the image down for inline previews.
	w := imgWidth - padding
	rightMargin := 100
	gap := 24 // gap between the name column and the data columns
	spreadW := 130
	revW := 160 // wider so hint "how long to expect to hold" fits comfortably
	bulkW := 130
	sellW := 130
	buyW := 130
	curW := 130

	spreadL := w - rightMargin - spreadW
	revL := spreadL - revW
	bulkL := revL - bulkW
	sellL := bulkL - sellW
	buyL := sellL - buyW
	curL := buyL - curW
	nameL := padding + 12

	return [7]column{
		{nameL, curL - gap},                  // Currency (left-aligned)
		{curL, curL + curW},                  // Current
		{buyL, buyL + buyW},                  // Buy
		{sellL, sellL + sellW},               // Sell
		{bulkL, bulkL + bulkW},               // Bulk
		{revL, revL + revW},                  // Reversion (half-life)
		{spreadL, spreadL + spreadW},         // Spread
	}
}

func drawText(dc *gg.Context, s string, x, y int) {
	dc.DrawString(s, float64(x), float64(y))
}

func drawTextRight(dc *gg.Context, s string, rightX, y int) {
	w, _ := dc.MeasureString(s)
	dc.DrawString(s, float64(rightX)-w, float64(y))
}

func fmtFloat(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

func fmtSpread(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

// fmtHalfLife renders an OU half-life as a compact duration ("9h", "1d 4h").
func fmtHalfLife(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	h := int(d.Hours())
	if h < 24 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dd %dh", h/24, h%24)
}

func displayOf(r Recommendation) string {
	if r.Display != "" {
		return r.Display
	}
	return r.Name
}

func spreadOf(r Recommendation) float64 {
	if r.BuyTarget <= 0 {
		return 0
	}
	return (r.SellTarget - r.BuyTarget) / r.BuyTarget * 100
}

func loadFace(size float64) (font.Face, error) {
	f, err := opentype.Parse(goFont)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: 0,
	})
}
