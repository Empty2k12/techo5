//go:build !dot

package display

import (
	"fmt"
	"image"
	"image/draw"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
)

// weatherShow is how long the weather page stays after the question that brought it up.
const weatherShow = 30 * time.Second

// weatherPage is today large on the left and the next days as columns on the right.
func (r *renderer) weatherPage(s scene) {
	r.cornerClock(s)
	r.text(r.small, "Weather", r.margin, r.margin+26, amber)

	days := s.forecast
	now := s.weather
	// Today: the reading, then the day's range.
	big := now.Temp
	if big == "" && len(days) > 0 {
		big = fmt.Sprintf("%.0f°", days[0].High)
	}
	r.text(r.clock, big, r.margin, 250, cream)
	cond := conditionWords(now.Condition)
	if cond == "" && len(days) > 0 {
		cond = conditionWords(days[0].Condition)
	}
	r.text(r.body, cond, r.margin, 305, dim)
	if len(days) > 0 {
		r.text(r.small, fmt.Sprintf("High %.0f°  Low %.0f°", days[0].High, days[0].Low), r.margin, 350, dim)
		if days[0].Rain >= 0 {
			r.text(r.small, fmt.Sprintf("Rain %d%%", days[0].Rain), r.margin, 390, dim)
		}
	}

	// The next five days as columns.
	if len(days) > 1 {
		left := r.w/2 - 20
		cols := min(5, len(days)-1)
		colW := (r.w - r.margin - left) / cols
		for i := 0; i < cols; i++ {
			d := days[i+1]
			x := left + i*colW
			name := d.When.Format("Mon")
			if d.When.IsZero() {
				name = fmt.Sprintf("+%d", i+1)
			}
			r.text(r.small, name, x+(colW-r.width(r.small, name))/2, 130, amber)
			hi := fmt.Sprintf("%.0f°", d.High)
			lo := fmt.Sprintf("%.0f°", d.Low)
			r.text(r.body, hi, x+(colW-r.width(r.body, hi))/2, 200, cream)
			r.text(r.small, lo, x+(colW-r.width(r.small, lo))/2, 245, dim)
			c := shortCondition(d.Condition)
			r.text(r.tiny, c, x+(colW-r.width(r.tiny, c))/2, 285, dim)
			if d.Rain > 0 {
				p := fmt.Sprintf("%d%%", d.Rain)
				r.text(r.tiny, p, x+(colW-r.width(r.tiny, p))/2, 315, dim)
			}
			draw.Draw(r.dst, image.Rect(x+8, 340, x+colW-8, 342), image.NewUniform(ember), image.Point{}, draw.Src)
		}
	} else if len(days) == 0 {
		msg := "No forecast yet: call the home_assistant action with a token"
		r.text(r.tiny, msg, r.w/2-20, 200, dim)
	}
}

// shortCondition is a word that fits a column.
func shortCondition(c string) string {
	switch c {
	case "partlycloudy":
		return "Part cloudy"
	case "lightning-rainy", "lightning":
		return "Storms"
	case "clear-night":
		return "Clear"
	case "pouring":
		return "Heavy rain"
	}
	return conditionWords(c)
}

// forecastDays is a type alias for the scene.
type forecastDays = []hass.Day
