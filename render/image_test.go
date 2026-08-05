package render

import "testing"

func TestColumnPositionsDontOverflow(t *testing.T) {
	cols := columnPositions()
	if cols[0].LeftX != 36 {
		t.Errorf("Currency LeftX = %d, want 36", cols[0].LeftX)
	}
	maxSpreadRight := 1280 - 24 - 100
	if cols[6].RightX > maxSpreadRight {
		t.Errorf("Spread right edge %d exceeds max %d", cols[6].RightX, maxSpreadRight)
	}
	// Columns should be ordered left-to-right with no overlap.
	for i := 1; i < len(cols); i++ {
		if cols[i].LeftX < cols[i-1].RightX {
			t.Errorf("column %d LeftX=%d overlaps previous RightX=%d",
				i, cols[i].LeftX, cols[i-1].RightX)
		}
	}
}

func TestTableRenders(t *testing.T) {
	recs := []Recommendation{
		{Name: "divine", Display: "Divine Orb", Current: 423.15, BuyTarget: 376.47, SellTarget: 422.94, BulkSell: 422.94},
		{Name: "chaos", Display: "Chaos Orb", Current: 44.50, BuyTarget: 40.00, SellTarget: 48.00, BulkSell: 51.00},
	}
	img := Table(recs, "runes", "PoE2 Currency Analysis")
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("zero-size image: %v", b)
	}
}
