package cli

import "strings"

// ExpandFlags splits clustered short flags and --key=value forms (swl2 expand_flags).
//
// Examples: -abc → -a -b -c; --foo=bar → --foo bar; -x=value → -x value
func ExpandFlags(argv []string) []string {
	var res []string
	for _, arg := range argv {
		if len(arg) > 0 && arg[0] == '-' && (len(arg) == 1 || arg[1] != '-') {
			// Short flag cluster: -abc or -x=value
			for i := 1; i < len(arg); i++ {
				if arg[i] == '=' {
					res = append(res, arg[i+1:])
					break
				}
				res = append(res, "-"+string(arg[i]))
			}
		} else if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			// Long flag with inline value: --key=value
			i := strings.IndexByte(arg, '=')
			res = append(res, arg[:i], arg[i+1:])
		} else {
			res = append(res, arg)
		}
	}
	return res
}

// CountVerboseFlags expands -v, -vv, -vvv into a verbosity count when passed alone.
// Returns the modified argv (with lone -v kept) and the accumulated verbose count.
func CountVerboseFlags(argv []string) ([]string, int) {
	var out []string
	verbose := 0
	for _, arg := range argv {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 1 {
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
