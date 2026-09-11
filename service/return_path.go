package service

import (
	"strings"

	"github.com/c1cadaBob/NexusTok/setting/system_setting"
)

func PaymentReturnURL(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + suffix
}
