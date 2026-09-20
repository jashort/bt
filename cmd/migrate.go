package cmd

import (
	"bt/internal"
	"fmt"
	"os"
	"path/filepath"
)

type MigrateCmd struct {
	Apply bool `help:"Perform the migration. Without this flag, only a dry-run report is printed."`
}

func (c *MigrateCmd) Run(ctx *Context) error {
	plan, err := internal.PlanMigration(ctx.DataDir)
	if err != nil {
		return err
	}

	for _, g := range plan.Groups {
		fmt.Printf("%s: %d entr%s\n", g.Source, len(g.Entries), plural(len(g.Entries)))
		for _, e := range g.Entries {
			extra := ""
			if e.Raw {
				extra = " (moved as-is)"
			} else {
				extra = fmt.Sprintf(" (location: %s)", e.LocationSource)
			}
			fmt.Printf("  %s -> %s%s\n", e.HeaderTime.Format("3:04 PM"), relPath(e.DestPath), extra)
		}
	}
	for _, s := range plan.Skipped {
		fmt.Printf("skipped unrecognized file: %s\n", s)
	}
	fmt.Printf("Summary: %d file(s), %d entr%s, %d source file(s) to delete\n",
		len(plan.Groups), plan.Total, plural(plan.Total), len(plan.Groups))

	if !c.Apply {
		if plan.Total > 0 {
			fmt.Println("Dry run - nothing written. Re-run with --apply to migrate.")
		}
		return nil
	}

	return internal.ApplyMigration(plan, func(format string, args ...any) {
		fmt.Printf(format, args...)
	})
}

// relPath shortens a path for display, dropping the working directory prefix when possible.
func relPath(p string) string {
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(wd, p); err == nil {
		return rel
	}
	return p
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
