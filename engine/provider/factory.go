// Copyright 2023 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"encoding/json"
	"fmt"

	"github.com/woodpecker-ci/woodpecker/woodpecker-go/woodpecker"

	"github.com/woodpecker-ci/autoscaler/engine/provider/oracle"
	"github.com/woodpecker-ci/autoscaler/engine/types"
)

func New(client woodpecker.Client, providerName, providerConfig string, poolID int) (types.Provider, error) {
	var p types.Provider

	switch providerName {
	case oracle.Name:
		var config oracle.Provider
		if err := json.Unmarshal([]byte(providerConfig), &config); err != nil {
			return nil, err
		}
		p = &config
	default:
		return nil, fmt.Errorf("provider '%s' not supported", providerName)
	}

	return p, nil
}
