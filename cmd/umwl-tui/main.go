// umwl-tui — full-screen importer of unmanaged workloads and IP lists into an Illumio PCE,
// on top of brian1917/workloader. Run it from the working folder of one Illumio account + PCE.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/roschereric/illumio-workloader-import-kit/internal/tui"
)

var version = "dev"

func main() {
	cfg := tui.Config{Version: version}
	flag.StringVar(&cfg.IPL, "ipl", "", "IP lists CSV (workloader ipl-import format)")
	flag.StringVar(&cfg.PCE, "pce", "", "PCE name in pce.yaml (workloader --pce)")
	flag.StringVar(&cfg.Priority, "priority", "", "only rows whose description carries [.. P<n> ..] for these priorities, e.g. 1 or 1,2")
	flag.StringVar(&cfg.WorkloaderBin, "workloader", "", "path to the workloader binary (default: ./workloader, then PATH)")
	flag.StringVar(&cfg.RunsDir, "runs", "./runs", "folder for run artefacts")
	flag.IntVar(&cfg.Chunk, "chunk", 20, "rows per wkld-import call")
	flag.BoolVar(&cfg.SetupOnly, "setup-only", false, "only verify/install workloader and the PCE connection")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: umwl-tui [flags] <proposed-workloads.csv>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVer {
		fmt.Println("umwl-tui", version)
		return
	}
	if flag.NArg() > 0 {
		cfg.CSV = flag.Arg(0)
	}
	if err := tui.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
