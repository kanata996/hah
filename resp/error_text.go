package resp

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const maxErrorStringBytes = 1024

// safeErrorString 读取错误文本，并对异常 Error() 实现做恢复。
func safeErrorString(err error) (message string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			message = "panic calling Error()"
			if errType := errorTypeName(err); errType != "" {
				message += " on " + errType
			}
			message += ": " + fmt.Sprint(recovered)
			message = limitErrorString(message)
		}
	}()

	return limitErrorString(err.Error())
}

func errorTypeName(err error) string {
	if err == nil {
		return ""
	}
	return reflect.TypeOf(err).String()
}

// limitErrorString 对错误文本做长度限制，避免错误文本本身过大。
// 截断位置会对齐到 UTF-8 rune 边界，避免产生非法 UTF-8 序列。
func limitErrorString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= maxErrorStringBytes {
		return trimmed
	}

	cut := maxErrorStringBytes
	for cut > 0 && !utf8.RuneStart(trimmed[cut]) {
		cut--
	}
	return trimmed[:cut] + "...(truncated)"
}
