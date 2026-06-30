package cli

import "github.com/ceymard/swl-go/internal/optparse"

// ExpandFlags splits clustered short flags and --key=value forms (swl2 expand_flags).
func ExpandFlags(argv []string) []string {
	return optparse.ExpandFlags(argv)
}

// CountVerboseFlags expands -v, -vv, -vvv into a verbosity count when passed alone.
func CountVerboseFlags(argv []string) ([]string, int) {
	var out []string
	verbose := 0
	for _, arg := range argv {
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
			allV := true
			for i := 1; i < len(arg); i++ {
				if arg[i] != 'v' {
					allV = false
					break
				}
			}
			if allV && arg != "-v" {
				verbose += len(arg) - 1
				continue
			}
		}
		if arg == "-v" || arg == "--verbose" {
			verbose++
			out = append(out, arg)
			continue
		}
		out = append(out, arg)
	}
	return out, verbose
}
