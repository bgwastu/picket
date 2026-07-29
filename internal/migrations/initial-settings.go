package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.Meta.HideControls = true
		settings.Logs.MinLevel = 4
		return app.Save(settings)
	}, nil)
}
