package optparse

import "strings"

// ExpandFlags splits clustered short flags and --key=value forms (swl2 expand_flags).
func ExpandFlags(argv []string) []string {
	var res []string
	for _, arg := range argv {
		if len(arg) > 0 && arg[0] == '-' && (len(arg) == 1 || arg[1] != '-') {
			for i := 1; i < len(arg); i++ {
				if arg[i] == '=' {
					res = append(res, arg[i+1:])
					break
				}
				res = append(res, "-"+string(arg[i]))
			}
		} else if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			i := strings.IndexByte(arg, '=')
			res = append(res, arg[:i], arg[i+1:])
		} else {
			res = append(res, arg)
		}
	}
	return res
}
