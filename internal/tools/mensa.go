package tools

import (
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/mensa"
)

var weekdayNames = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

type mensaDay struct {
	Date    string       `json:"date"`
	Weekday string       `json:"weekday"`
	Dishes  []mensa.Dish `json:"dishes"`
}

func mensaTools() []*Tool {
	return []*Tool{
		{Name: "mensa_menu", Kind: Read, Desc: "Speiseplan der NORDAKADEMIE-Mensa: Gerichte je Tag mit Preis, Kennzeichnung (vegetarisch, vegan, Rind …), Allergenen und Zusatzstoffen; aktuelle Woche und bis zu zwei folgende.",
			Params: []Param{{Name: "weeks", Type: "integer", Desc: "Wochen ab heute (1–3, Standard 2)"}},
			Run: func(a *app.App, args Args) (any, error) {
				n, err := args.Int("weeks", 2)
				if err != nil {
					return nil, err
				}
				ds, err := a.Mensa().Menu(int(n))
				if err != nil {
					return nil, err
				}
				var days []mensaDay
				for _, d := range ds {
					if len(days) == 0 || days[len(days)-1].Date != d.Date {
						wd := ""
						if t, err := time.Parse("2006-01-02", d.Date); err == nil {
							wd = weekdayNames[t.Weekday()]
						}
						days = append(days, mensaDay{Date: d.Date, Weekday: wd})
					}
					days[len(days)-1].Dishes = append(days[len(days)-1].Dishes, d)
				}
				if days == nil {
					days = []mensaDay{}
				}
				return map[string]any{"days": days}, nil
			}},
		{Name: "mensa_account", Kind: Read, Desc: "Mensakarte: aktuelles Guthaben und die Buchungen der letzten Tage (Essen, Getränke, Aufladungen, Pfand).",
			Params: []Param{{Name: "days", Type: "integer", Desc: "Buchungen der letzten n Tage (Standard 60)"}},
			Run: func(a *app.App, args Args) (any, error) {
				n, err := args.Int("days", 60)
				if err != nil {
					return nil, err
				}
				now := a.Now().In(a.Zone)
				acc, err := a.Mensa().Account(now.AddDate(0, 0, -int(n)), now)
				if err != nil {
					return nil, err
				}
				return acc, nil
			}},
		{Name: "mensa_spending", Kind: Read, Desc: "Wie viel insgesamt für Essen und Getränke in der Mensa ausgegeben wurde (ohne Aufladungen und Pfand), mit Anzahl der Menüs und Monatsverlauf.",
			Run: func(a *app.App, args Args) (any, error) {
				now := a.Now().In(a.Zone)
				acc, err := a.Mensa().Account(time.Date(2010, 1, 1, 0, 0, 0, 0, a.Zone), now)
				if err != nil {
					return nil, err
				}
				return map[string]any{"balance": acc.Balance, "spending": mensa.Spend(acc.Bookings)}, nil
			}},
	}
}
