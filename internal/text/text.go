// Copyright 2025 The Reginald Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package text defines general text utilities.
package text

import "strings"

// Wrap wraps the given string to the given width. It wraps each paragraph,
// separated by two newlines, separately.
func Wrap(s string, width int) string {
	var sb strings.Builder

	for p := range strings.SplitSeq(s, "\n\n") {
		words := strings.Fields(p)
		l := 0

		for i, w := range words {
			addForSpace := 0

			if l > 0 {
				addForSpace = 1
			}

			if l+len(w)+addForSpace > width {
				sb.WriteByte('\n')

				l = 0
			}

			if l > 0 {
				sb.WriteByte(' ')

				l++
			}

			sb.WriteString(w)

			l += len(w)

			if i == len(words)-1 {
				sb.WriteString("\n\n")
			}
		}
	}

	result := sb.String()

	if strings.HasSuffix(result, "\n\n") {
		result = result[:len(result)-1]
	}

	return result
}
