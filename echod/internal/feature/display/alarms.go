//go:build !dot && !spot

package display

import (
	"log/slog"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/alarm"
	"github.com/HuskerMinion/techo5/echod/internal/feature/timer"
)

// alarmDraft is an alarm being set on the Alarms tab: a new one, or a copy of one being changed.
type alarmDraft struct {
	alarm     config.Alarm
	isNew     bool
	deleteArm time.Time // the first of the two taps Delete wants
}

// ringState is what is ringing, for the ringing page.
type ringState struct {
	timer     string // name of a ringing timer
	alarm     *alarm.Ring
	preview   bool
	snoozable bool
}

func (r ringState) any() bool { return r.timer != "" || r.alarm != nil || r.preview }

// ringing reads what is sounding now.
func (d *Display) ringing(now time.Time) ringState {
	var st ringState
	if name, ok := timer.Get().RingingName(); ok {
		st.timer = name
	}
	st.alarm = alarm.Get().View(now).Ringing
	st.snoozable = st.alarm != nil
	d.mu.Lock()
	if now.Before(d.ringPreview) && !st.any() {
		st.preview, st.snoozable = true, true
		st.alarm = &alarm.Ring{Label: "Wake up", At: now}
	}
	d.mu.Unlock()
	return st
}

// PreviewRing shows the ringing page for a while with nothing sounding, to look at it.
func (d *Display) PreviewRing(for_ time.Duration) {
	d.mu.Lock()
	d.ringPreview = time.Now().Add(for_)
	d.mu.Unlock()
	d.wake()
}

// ringTap is a finger on the ringing page: Stop on the left half of the buttons, Snooze on the right
// when there is one to snooze.
func (d *Display) ringTap(x, y int, st ringState) {
	if d.r == nil || y < ringButtonsTop-20 {
		return
	}
	d.mu.Lock()
	d.ringPreview = time.Time{}
	d.mu.Unlock()
	if st.snoozable && x >= d.r.w/2 {
		if !alarm.Get().Snooze() {
			slog.Debug("snooze with nothing ringing")
		}
		timer.Get().Stop()
		return
	}
	timer.Get().Stop()
	alarm.Get().Stop()
}

// EditNewAlarm opens the alarm editor on a new alarm at the next whole hour.
func (d *Display) EditNewAlarm() {
	next := time.Now().Add(time.Hour)
	d.mu.Lock()
	d.draft = &alarmDraft{isNew: true, alarm: config.Alarm{Hour: next.Hour(), Minute: 0, Days: config.DaysOnce, On: true}}
	d.mu.Unlock()
	d.wake()
}

// alarmRows is the list the Alarms tab shows, in order: the device's alarms, the helpers followed, and
// the row that adds one.
type alarmRow struct {
	snoozed  *alarm.Upcoming
	local    *config.Alarm
	followed *alarm.Followed
	add      bool
}

func alarmRows(v alarm.View) []alarmRow {
	var rows []alarmRow
	for i := range v.Snoozed {
		rows = append(rows, alarmRow{snoozed: &v.Snoozed[i]})
	}
	for i := range v.Local {
		rows = append(rows, alarmRow{local: &v.Local[i]})
	}
	for i := range v.Followed {
		rows = append(rows, alarmRow{followed: &v.Followed[i]})
	}
	return append(rows, alarmRow{add: true})
}

// alarmsTap is a row of the Alarms tab, or of the editor while one is open.
func (d *Display) alarmsTap(h hit, page int) {
	d.mu.Lock()
	var draft *alarmDraft
	if d.draft != nil {
		c := *d.draft
		draft = &c
	}
	d.mu.Unlock()
	if draft != nil {
		keep := d.draftTap(h, draft)
		d.mu.Lock()
		if keep {
			d.draft = draft
		} else {
			d.draft = nil
		}
		d.mu.Unlock()
		return
	}
	rows := alarmRows(alarm.Get().View(time.Now()))
	i, ok := d.listTap(len(rows), page, h.row)
	if !ok {
		return
	}
	switch row := rows[i]; {
	case row.snoozed != nil && h.button == 2:
		alarm.Get().CancelSnoozes()
	case row.add && h.button != 0:
		d.EditNewAlarm()
	case row.local != nil && h.button == 1:
		d.mu.Lock()
		d.draft = &alarmDraft{alarm: *row.local}
		d.mu.Unlock()
	case row.local != nil && h.button == 2:
		a := *row.local
		a.On = !a.On
		if err := alarm.Get().Put(a); err != nil {
			slog.Warn("saving an alarm failed", "err", err)
		}
	}
}

// Editor rows, in order.
const (
	editRowHour = iota
	editRowMinute
	editRowDays
	editRowRepeat
	editRowSave
	editRowDelete
	editRowBack
)

// dayChips is how the Days row lays out its seven days: from x0, each chipW wide.
func (r *renderer) dayChips() (x0, chipW int) {
	x0 = r.sheetLeft() + 150
	return x0, (r.w - sheetPad - x0) / 7
}

// repeats are the choices the Repeat row walks through.
var repeats = []uint8{config.DaysOnce, config.DaysEvery, config.DaysWeekdays, config.DaysWeekends}

// draftTap changes the draft it is given, and reports whether the editor stays open.
func (d *Display) draftTap(h hit, draft *alarmDraft) bool {
	a := &draft.alarm
	switch h.row {
	case editRowHour:
		switch h.button {
		case 1:
			a.Hour = (a.Hour + 23) % 24
		case 2:
			a.Hour = (a.Hour + 1) % 24
		}
	case editRowMinute:
		switch h.button {
		case 1:
			a.Minute = ((a.Minute+4)/5*5 + 55) % 60
		case 2:
			a.Minute = (a.Minute/5*5 + 5) % 60
		}
	case editRowDays:
		if d.r == nil {
			return true
		}
		x0, w := d.r.dayChips()
		if h.x >= x0 && h.x < x0+7*w {
			a.Days ^= 1 << ((h.x - x0) / w)
		}
	case editRowRepeat:
		if h.button == 2 {
			next := 0
			for i, days := range repeats {
				if days == a.Days {
					next = i + 1
				}
			}
			a.Days = repeats[next%len(repeats)]
		}
	case editRowSave:
		if h.button == 2 {
			a.On = true
			var err error
			if draft.isNew {
				_, err = alarm.Get().Set(a.Hour, a.Minute, a.Days, a.Label)
			} else {
				err = alarm.Get().Put(*a)
			}
			if err != nil {
				slog.Warn("saving an alarm failed", "err", err)
				return true
			}
			return false
		}
	case editRowDelete:
		if h.button != 2 {
			return true
		}
		if draft.isNew {
			return false // Cancel
		}
		if !draft.deleteArm.IsZero() && time.Since(draft.deleteArm) < restartWindow {
			if err := alarm.Get().Delete(a.ID); err != nil {
				slog.Warn("deleting an alarm failed", "err", err)
			}
			return false
		}
		draft.deleteArm = time.Now()
	case editRowBack:
		if h.button == 2 {
			return false
		}
	}
	return true
}
