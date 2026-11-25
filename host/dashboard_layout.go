package main

const (
	dashboardLayoutBaseWidth = 906
	dashboardMinimumWidth    = 760
)

type dashboardBounds struct {
	x, y, width, height int
}

func dashboardControlBounds(x, y, width, height, clientWidth int) dashboardBounds {
	if clientWidth < dashboardMinimumWidth {
		clientWidth = dashboardMinimumWidth
	}
	return dashboardBounds{
		x:      x * clientWidth / dashboardLayoutBaseWidth,
		y:      y,
		width:  max(1, width*clientWidth/dashboardLayoutBaseWidth),
		height: height,
	}
}

func dashboardTabBounds(index, clientWidth int) dashboardBounds {
	if clientWidth < dashboardMinimumWidth {
		clientWidth = dashboardMinimumWidth
	}
	const margin, gap = 20, 8
	width := (clientWidth - 2*margin - 5*gap) / 6
	return dashboardBounds{
		x:      margin + index*(width+gap),
		y:      76,
		width:  width,
		height: 34,
	}
}

func dashboardCardBounds(clientWidth int) (dashboardBounds, dashboardBounds) {
	if clientWidth < dashboardMinimumWidth {
		clientWidth = dashboardMinimumWidth
	}
	const margin, gap = 25, 20
	width := (clientWidth - 2*margin - gap) / 2
	return dashboardBounds{
		x: margin, y: 135, width: width, height: 247,
	}, dashboardBounds{
		x: margin + width + gap, y: 135, width: width, height: 247,
	}
}
