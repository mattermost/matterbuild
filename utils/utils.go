// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package utils

import (
	"fmt"
	"time"
)

func MilisecsToMinutes(value int64) string {
	str := fmt.Sprintf("%v", time.Duration(value)*time.Millisecond)
	return str
}
