# legacy/ — plain-terminal loaders (superseded by umwl-tui)

`umwl_loader.py` (interactive, sequential prompts) and `reconcile_umwl.py` (non-interactive IP reconciliation) were the
first implementation of the kit. They are kept for environments where the Go binary cannot be used (no way to run an
unsigned binary, exotic OS) and as a readable reference of the flow. They implement the same steps and the same
workloader flags as `umwl-tui` (including `--match name` / `--match href`), but new features land in `umwl-tui` first.

```bash
python3 legacy/umwl_loader.py --setup-only
python3 legacy/umwl_loader.py <customer>-umwl-import.csv --ipl <customer>-ipl-import.csv --priority 1
python3 legacy/reconcile_umwl.py pce-workloads.csv <customer>-umwl-import.csv     # reconciliation only, no writes
```

Run them from the working folder (one per Illumio account + PCE), never from inside this folder.
