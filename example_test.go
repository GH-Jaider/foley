package foley_test

import (
	"context"
	"log"
	"regexp"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/key"
)

// Example records a short interactive session into a GIF and an MP4 from
// the same take. It compiles with the test suite but does not run (it
// needs a live application).
func Example() {
	rec, err := foley.New(foley.Options{
		Command: []string{"my-tui"},
		Cols:    100,
		Rows:    30,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = rec.Close() }()

	ctx := context.Background()
	must(rec.Type(ctx, "hello", 50*time.Millisecond))
	must(rec.Press(ctx, key.Key{Name: key.NameEnter}, 0))
	must(rec.WaitText(ctx, regexp.MustCompile(`done`), 10*time.Second))
	must(rec.Sleep(ctx, 2*time.Second))
	must(rec.Output(ctx, "demo.gif"))
	must(rec.Output(ctx, "demo.mp4"))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
