package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const systemsCollectionID = "2hz5ncl8tizk5nx"

var publicRule = ""

func init() {
	m.Register(func(app core.App) error {
		collections := []*core.Collection{
			systemsCollection(),
			systemStatsCollection(),
			containersCollection(),
			containerStatsCollection(),
			systemDetailsCollection(),
			systemdServicesCollection(),
			alertsCollection(),
			alertsHistoryCollection(),
			notificationSettingsCollection(),
		}
		for _, collection := range collections {
			if err := app.Save(collection); err != nil {
				return err
			}
		}

		settings := core.NewRecord(collections[len(collections)-1])
		settings.Id = "globalsettings1"
		settings.Set("settings", map[string]any{
			"telegramBotToken": "",
			"telegramUserIds":  []string{},
		})
		return app.Save(settings)
	}, nil)
}

func baseCollection(name, id string, fields ...core.Field) *core.Collection {
	collection := core.NewBaseCollection(name, id)
	collection.Fields.Add(fields...)
	return collection
}

func publicRead(collection *core.Collection) *core.Collection {
	collection.ListRule = &publicRule
	collection.ViewRule = &publicRule
	return collection
}

func publicCRUD(collection *core.Collection) *core.Collection {
	publicRead(collection)
	collection.CreateRule = &publicRule
	collection.UpdateRule = &publicRule
	collection.DeleteRule = &publicRule
	return collection
}

func timestamps() []core.Field {
	return []core.Field{
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	}
}

func systemRelation(required bool) *core.RelationField {
	return &core.RelationField{Name: "system", CollectionId: systemsCollectionID, CascadeDelete: true, Required: required, MaxSelect: 1}
}

func systemsCollection() *core.Collection {
	collection := baseCollection("systems", systemsCollectionID,
		&core.TextField{Name: "name", Required: true},
		&core.SelectField{Name: "status", Values: []string{"up", "down", "paused", "pending"}},
		&core.JSONField{Name: "info", MaxSize: 2_000_000},
		&core.TextField{Name: "token", Hidden: true, Required: true, Min: 20, Max: 255, AutogeneratePattern: "[a-zA-Z0-9]{32}"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	collection.AddIndex("idx_systems_status", false, "status", "")
	collection.AddIndex("idx_systems_token", true, "token", "")
	return publicCRUD(collection)
}

func statsCollection(name, id string) *core.Collection {
	fields := []core.Field{
		systemRelation(true),
		&core.JSONField{Name: "stats", Required: true, MaxSize: 2_000_000},
		&core.SelectField{Name: "type", Required: true, Values: []string{"1m", "10m", "20m", "120m", "480m"}},
	}
	fields = append(fields, timestamps()...)
	collection := publicRead(baseCollection(name, id, fields...))
	collection.AddIndex("idx_"+name+"_system_type_created", false, "system, type, created", "")
	return collection
}

func systemStatsCollection() *core.Collection {
	return statsCollection("system_stats", "ej9oowivz8b2mht")
}

func containerStatsCollection() *core.Collection {
	return statsCollection("container_stats", "juohu4jipgc13v7")
}

func containersCollection() *core.Collection {
	collection := publicRead(baseCollection("containers", "pbc_1864144027",
		systemRelation(false),
		&core.TextField{Name: "name"},
		&core.TextField{Name: "status"},
		&core.NumberField{Name: "health"},
		&core.NumberField{Name: "cpu"},
		&core.NumberField{Name: "memory"},
		&core.NumberField{Name: "net"},
		&core.TextField{Name: "image"},
		&core.TextField{Name: "ports"},
		&core.NumberField{Name: "updated", Required: true, OnlyInt: true},
	))
	collection.AddIndex("idx_containers_updated", false, "updated", "")
	collection.AddIndex("idx_containers_system", false, "system", "")
	return collection
}

func systemDetailsCollection() *core.Collection {
	collection := publicRead(baseCollection("system_details", "pbc_3116237454",
		systemRelation(true),
		&core.TextField{Name: "hostname"},
		&core.NumberField{Name: "os"},
		&core.TextField{Name: "os_name"},
		&core.TextField{Name: "kernel"},
		&core.TextField{Name: "cpu"},
		&core.TextField{Name: "arch"},
		&core.NumberField{Name: "cores"},
		&core.NumberField{Name: "threads"},
		&core.NumberField{Name: "memory"},
		&core.BoolField{Name: "podman"},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	))
	collection.AddIndex("idx_system_details_system", true, "system", "")
	return collection
}

func systemdServicesCollection() *core.Collection {
	collection := publicRead(baseCollection("systemd_services", "pbc_3494996990",
		&core.TextField{Name: "name"},
		systemRelation(false),
		&core.NumberField{Name: "state", OnlyInt: true},
		&core.NumberField{Name: "sub", OnlyInt: true},
		&core.NumberField{Name: "cpu"},
		&core.NumberField{Name: "cpuPeak"},
		&core.NumberField{Name: "memory"},
		&core.NumberField{Name: "memPeak"},
		&core.NumberField{Name: "updated"},
	))
	collection.AddIndex("idx_systemd_services_system", false, "system", "")
	collection.AddIndex("idx_systemd_services_updated", false, "updated", "")
	return collection
}

func alertsCollection() *core.Collection {
	fields := []core.Field{
		systemRelation(true),
		&core.SelectField{Name: "name", Required: true, Values: []string{"Status", "CPU", "Memory", "Disk", "Bandwidth", "GPU", "LoadAvg1", "LoadAvg5", "LoadAvg15"}},
		&core.NumberField{Name: "value"},
		&core.NumberField{Name: "min", OnlyInt: true},
		&core.BoolField{Name: "triggered"},
	}
	fields = append(fields, timestamps()...)
	collection := publicCRUD(baseCollection("alerts", "elngm8x1l60zi2v", fields...))
	collection.AddIndex("idx_alerts_system_name", true, "system, name", "")
	return collection
}

func alertsHistoryCollection() *core.Collection {
	collection := publicRead(baseCollection("alerts_history", "pbc_1697146157",
		systemRelation(true),
		&core.TextField{Name: "alert_id"},
		&core.TextField{Name: "name", Required: true},
		&core.NumberField{Name: "value"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.DateField{Name: "resolved"},
	))
	collection.DeleteRule = &publicRule
	collection.AddIndex("idx_alerts_history_created", false, "created", "")
	return collection
}

func notificationSettingsCollection() *core.Collection {
	collection := publicRead(baseCollection("notification_settings", "notifications01",
		&core.JSONField{Name: "settings", Required: true, MaxSize: 2_000_000},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	))
	collection.UpdateRule = &publicRule
	return collection
}
