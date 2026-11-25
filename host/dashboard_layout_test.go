package main

import "testing"

func TestDashboardResponsiveBoundsFitClient(t *testing.T) {
	for _, width := range []int{dashboardMinimumWidth, 906, 1200, 1600} {
		for i := 0; i < 6; i++ {
			tab := dashboardTabBounds(i, width)
			if tab.x < 0 || tab.width <= 0 || tab.x+tab.width > width {
				t.Fatalf("tab %d at width %d exceeds client: %+v", i, width, tab)
			}
			if i > 0 {
				previous := dashboardTabBounds(i-1, width)
				if tab.x < previous.x+previous.width {
					t.Fatalf("tabs overlap at width %d: %+v, %+v", width, previous, tab)
				}
			}
		}

		left, right := dashboardCardBounds(width)
		bottomRight := dashboardRightBottomCardBounds(width)
		if left.x+left.width > right.x || right.x+right.width > width {
			t.Fatalf("dashboard cards exceed client at width %d: %+v, %+v", width, left, right)
		}
		if bottomRight.x != right.x || bottomRight.width != right.width || bottomRight.y+bottomRight.height != left.y+left.height {
			t.Fatalf("right bottom card misaligned at width %d: left=%+v bottomRight=%+v", width, left, bottomRight)
		}
		if left.width < 320 {
			t.Fatalf("preview card too narrow at width %d: %+v", width, left)
		}
		full := dashboardFullCardBounds(width, 126, 250)
		if full.x+full.width > width || full.width < 700 {
			t.Fatalf("full card invalid at width %d: %+v", width, full)
		}

		control := dashboardControlBounds(650, 200, 210, 30, width)
		if control.x < 0 || control.width <= 0 || control.x+control.width > width {
			t.Fatalf("control exceeds client at width %d: %+v", width, control)
		}
	}
}
