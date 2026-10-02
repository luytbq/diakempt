package tidy

import "fmt"

// OrderKept checks relative order against the original geometry: no two solid
// shapes may swap sides on either axis. Two shapes that were level may end up
// apart, and two that were apart may end up level, but never reversed.
func (d *Diagram) OrderKept() error {
	solid := d.solidNodes()
	for i := 0; i < len(solid); i++ {
		for j := i + 1; j < len(solid); j++ {
			a, b := solid[i], solid[j]
			for _, ax := range []axis{axisX, axisY} {
				before := center(a.Orig, ax) - center(b.Orig, ax)
				after := center(a.Box, ax) - center(b.Box, ax)
				if (before > sameLine && after < -sameLine) || (before < -sameLine && after > sameLine) {
					name := "x"
					if ax == axisY {
						name = "y"
					}
					return fmt.Errorf("%s and %s swapped on %s", a.V.ID, b.V.ID, name)
				}
			}
		}
	}
	return nil
}
