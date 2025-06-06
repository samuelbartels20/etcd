// Copyright 2021 The etcd Authors
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

package etcdutl

import (
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"

	"go.etcd.io/etcd/etcdutl/v3/snapshot"
)

type tablePrinter struct{ printer }

func (tp *tablePrinter) DBStatus(r snapshot.Status) {
	hdr, rows := makeDBStatusTable(r)
	cfgBuilder := tablewriter.NewConfigBuilder().WithRowAlignment(tw.AlignRight)
	table := tablewriter.NewTable(os.Stdout, tablewriter.WithConfig(cfgBuilder.Build()))
	table.Header(hdr)
	for _, row := range rows {
		table.Append(row)
	}
	table.Render()
}

func (tp *tablePrinter) DBHashKV(r HashKV) {
	hdr, rows := makeDBHashKVTable(r)
	cfgBuilder := tablewriter.NewConfigBuilder().WithRowAlignment(tw.AlignRight)
	table := tablewriter.NewTable(os.Stdout, tablewriter.WithConfig(cfgBuilder.Build()))
	table.Header(hdr)
	for _, row := range rows {
		table.Append(row)
	}
	table.Render()
}
