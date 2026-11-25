package main

func weatherDescription(code int) string {
	switch code {
	case 0:
		return "Clear"
	case 1:
		return "Mostly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45:
		return "Fog"
	case 48:
		return "Rime fog"
	case 51:
		return "Light drizzle"
	case 53:
		return "Drizzle"
	case 55:
		return "Heavy drizzle"
	case 56:
		return "Light freezing drizzle"
	case 57:
		return "Heavy freezing drizzle"
	case 61:
		return "Light rain"
	case 63:
		return "Rain"
	case 65:
		return "Heavy rain"
	case 66:
		return "Light freezing rain"
	case 67:
		return "Heavy freezing rain"
	case 71:
		return "Light snow"
	case 73:
		return "Snow"
	case 75:
		return "Heavy snow"
	case 77:
		return "Snow grains"
	case 80:
		return "Light rain showers"
	case 81:
		return "Rain showers"
	case 82:
		return "Heavy rain showers"
	case 85:
		return "Snow showers"
	case 86:
		return "Heavy snow showers"
	case 95:
		return "Thunderstorm"
	case 96:
		return "Thunderstorm with hail"
	case 99:
		return "Thunderstorm with heavy hail"
	default:
		return "Unknown"
	}
}
