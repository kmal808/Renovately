package main

import (
	"fmt"
	"log"
	"os"

	"reno/internal/handlers"
	"reno/internal/seed"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	_ "reno/migrations"
)

func main() {
	app := pocketbase.New()

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/static/{path...}", apis.Static(os.DirFS("public"), false))

		handlers.New(app).Register(se.Router)

		return se.Next()
	})

	// demo data seeder: reno seed demo@reno.local demo1234
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "seed [email] [password]",
		Short: "Create a demo user and a fully populated kitchen remodel project",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Bootstrap(); err != nil {
				return err
			}
			if err := app.RunAllMigrations(); err != nil {
				return err
			}
			if err := seed.Run(app, args[0], args[1]); err != nil {
				return err
			}
			fmt.Println("Seeded demo project. Log in with:", args[0])
			return nil
		},
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
