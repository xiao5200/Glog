package parser

import (
	"fmt"

	"glog/internal/errs"
)

func formatUnknownError(format string) error {
	return fmt.Errorf("%w: 不支持的格式 %q", errs.ErrParseFormatUnknown, format)
}

func parseFailedError(templateID, reason string) error {
	if templateID == "" {
		return fmt.Errorf("%w: %s", errs.ErrParseFailed, reason)
	}
	return fmt.Errorf("%w: 模板 %q: %s", errs.ErrParseFailed, templateID, reason)
}
