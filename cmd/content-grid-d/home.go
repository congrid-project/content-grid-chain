package main

import "strings"

func resolveHomeArg(args []string, defaultHome string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--home" {
			if i+1 < len(args) && args[i+1] != "" {
				return args[i+1]
			}
			return defaultHome
		}
		if strings.HasPrefix(arg, "--home=") {
			if home := strings.TrimPrefix(arg, "--home="); home != "" {
				return home
			}
			return defaultHome
		}
	}
	return defaultHome
}
