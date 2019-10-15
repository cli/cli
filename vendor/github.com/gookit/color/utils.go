package color

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// IsConsole 判断 w 是否为 stderr、stdout、stdin 三者之一
func IsConsole(out io.Writer) bool {
	o, ok := out.(*os.File)
	if !ok {
		return false
	}

	return o == os.Stdout || o == os.Stderr || o == os.Stdin
}

// IsMSys msys(MINGW64) 环境，不一定支持颜色
func IsMSys() bool {
	// like "MSYSTEM=MINGW64"
	if len(os.Getenv("MSYSTEM")) > 0 {
		return true
	}

	return false
}

// IsSupportColor check current console is support color.
//
// Supported:
// 	linux, mac, or windows's ConEmu, Cmder, putty, git-bash.exe
// Not support:
// 	windows cmd.exe, powerShell.exe
func IsSupportColor() bool {
	// "TERM=xterm"  support color
	// "TERM=xterm-vt220" support color
	// "TERM=xterm-256color" support color
	// "TERM=cygwin" don't support color
	if strings.Contains(os.Getenv("TERM"), "xterm") {
		return true
	}

	// like on ConEmu software, e.g "ConEmuANSI=ON"
	if os.Getenv("ConEmuANSI") == "ON" {
		return true
	}

	// like on ConEmu software, e.g "ANSICON=189x2000 (189x43)"
	if os.Getenv("ANSICON") != "" {
		return true
	}

	return false
}

// IsSupport256Color render
func IsSupport256Color() bool {
	// "TERM=xterm-256color"
	return strings.Contains(os.Getenv("TERM"), "256color")
}

// its Win system. linux windows darwin
// func isWindows() bool {
// 	return runtime.GOOS == "windows"
// }

func doPrint(code string, colors []Color, str string) {
	if isLikeInCmd {
		winPrint(str, colors...)
	} else {
		_, _ = fmt.Fprint(output, RenderString(code, str))
	}
}

func doPrintln(code string, colors []Color, args []interface{}) {
	str := formatArgsForPrintln(args)
	if isLikeInCmd {
		winPrintln(str, colors...)
	} else {
		_, _ = fmt.Fprintln(output, RenderString(code, str))
	}
}

func doPrintV2(code, str string) {
	if isLikeInCmd {
		renderColorCodeOnCmd(func() {
			_, _ = fmt.Fprint(output, RenderString(code, str))
		})
	} else {
		_, _ = fmt.Fprint(output, RenderString(code, str))
	}
}

func doPrintlnV2(code string, args []interface{}) {
	str := formatArgsForPrintln(args)
	if isLikeInCmd {
		renderColorCodeOnCmd(func() {
			_, _ = fmt.Fprintln(output, RenderString(code, str))
		})
	} else {
		_, _ = fmt.Fprintln(output, RenderString(code, str))
	}
}

func stringToArr(str, sep string) (arr []string) {
	str = strings.TrimSpace(str)
	if str == "" {
		return
	}

	ss := strings.Split(str, sep)
	for _, val := range ss {
		if val = strings.TrimSpace(val); val != "" {
			arr = append(arr, val)
		}
	}

	return
}

// if use Println, will add spaces for each arg
func formatArgsForPrintln(args []interface{}) (message string) {
	if ln := len(args); ln == 0 {
		message = ""
	} else if ln == 1 {
		message = fmt.Sprint(args[0])
	} else {
		message = fmt.Sprintln(args...)
		// clear last "\n"
		message = message[:len(message)-1]
	}
	return
}
