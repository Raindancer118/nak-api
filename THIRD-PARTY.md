# Third-party software

The `nak` binary statically links these Go modules (license verified from each module's LICENSE file):

| Module | License |
|---|---|
| github.com/mark3labs/mcp-go | MIT |
| github.com/google/jsonschema-go | MIT |
| github.com/spf13/cast | MIT |
| github.com/spf13/cobra | Apache-2.0 |
| github.com/santhosh-tekuri/jsonschema/v6 | Apache-2.0 |
| github.com/spf13/pflag | BSD-3-Clause |
| github.com/google/uuid | BSD-3-Clause |
| github.com/ledongthuc/pdf | BSD-3-Clause |
| github.com/yosida95/uritemplate/v3 | BSD-3-Clause |
| golang.org/x/net, x/sys, x/term, x/text | BSD-3-Clause |

The Go standard library is BSD-3-Clause. Full license texts ship with each module.
If installed, Poppler's `pdftotext` is called as an external program for PDF text extraction (not linked).
