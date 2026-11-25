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
		x: margin, y: 126, width: width, height: 422,
	}, dashboardBounds{
		x: margin + width + gap, y: 126, width: width, height: 244,
	}
}

func dashboardRightBottomCardBounds(clientWidth int) dashboardBounds {
	_, rightTop := dashboardCardBounds(clientWidth)
	return dashboardBounds{
		x:      rightTop.x,
		y:      rightTop.y + rightTop.height + 16,
		width:  rightTop.width,
		height: 162,
	}
}

func dashboardFullCardBounds(clientWidth, y, height int) dashboardBounds {
	if clientWidth < dashboardMinimumWidth {
		clientWidth = dashboardMinimumWidth
	}
	return dashboardBounds{
		x:      25,
		y:      y,
		width:  clientWidth - 50,
		height: height,
	}
}
