//
// Copyright (c) 2016-2019 The Aurora Authors. All rights reserved.
// This program is free software. It comes without any warranty,
// to the extent permitted by applicable law. You can redistribute
// it and/or modify it under the terms of the Do What The Fuck You
// Want To Public License, Version 2, as published by Sam Hocevar.
// See LICENSE file for more details or see below.
//

//
//        DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
//                    Version 2, December 2004
//
// Copyright (C) 2004 Sam Hocevar <sam@hocevar.net>
//
// Everyone is permitted to copy and distribute verbatim or modified
// copies of this license document, and changing it is allowed as long
// as the name is changed.
//
//            DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
//   TERMS AND CONDITIONS FOR COPYING, DISTRIBUTION AND MODIFICATION
//
//  0. You just DO WHAT THE FUCK YOU WANT TO.
//

package aurora

import (
	"fmt"
)

// Sprintf allows to use Value as format. For example
//
//    v := Sprintf(Red("total: +3.5f points"), Blue(3.14))
//
// In this case "total:" and "points" will be red, but
// 3.14 will be blue. But, in another example
//
//    v := Sprintf(Red("total: +3.5f points"), 3.14)
//
// full string will be red. And no way to clear 3.14 to
// default format and color
func Sprintf(format interface{}, args ...interface{}) string {
	switch ft := format.(type) {
	case string:
		return fmt.Sprintf(ft, args...)
	case Value:
		for i, v := range args {
			if val, ok := v.(Value); ok {
				args[i] = val.setTail(ft.Color())
				continue
			}
		}
		return fmt.Sprintf(ft.String(), args...)
	}
	// unknown type of format (we hope it's a string)
	return fmt.Sprintf(fmt.Sprint(format), args...)
}
