package ui

import (
	"slices"

	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
)

var (
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorHeading).Padding(0, 1)
	tableCellStyle   = lipgloss.NewStyle().Padding(0, 1)
)

// Table は罫線付きの表を組み立てる
type Table struct {
	headers []string
	rows    [][]string

	// maxWidth は表全体の上限幅 0 以下なら無制限(D22)
	maxWidth int
	// dropOrder は上限幅に収まらないときに落とす列の名前 この順に落とす(D22)
	dropOrder []string
	// truncate は列名ごとの切り詰め上限桁数(D22)
	truncate map[string]int
}

// NewTable はヘッダーを指定して Table を作る
func NewTable(headers ...string) *Table {
	return &Table{headers: headers}
}

// AddRow は 1 行分のセルを追加する
func (t *Table) AddRow(cells ...string) *Table {
	t.rows = append(t.rows, cells)
	return t
}

// MaxWidth は表全体の上限幅を指定する 未指定(0 以下)なら無制限のまま自然幅で描く(D22)
func (t *Table) MaxWidth(n int) *Table {
	t.maxWidth = n
	return t
}

// DropWhenNarrow は 自然幅が MaxWidth を超えるときにこの順で列を落とす対象を指定する
// 落として収まった時点で止まる 全部落としてもまだ超えるなら残りの列は折り返しに任せる(D22)
func (t *Table) DropWhenNarrow(headers ...string) *Table {
	t.dropOrder = headers
	return t
}

// Truncate は指定した列のセルを maxCells 桁で切り詰め 超えた分を "…" に置き換える
// 折り返しではなく切り詰めるので ID や ARN など横に伸びやすい列向け 桁数は lipgloss.Width 相当(全角対応)で数える(D22)
func (t *Table) Truncate(header string, maxCells int) *Table {
	if t.truncate == nil {
		t.truncate = make(map[string]int)
	}
	t.truncate[header] = maxCells
	return t
}

// Render は罫線付きの表を文字列として描画する
// MaxWidth を指定していて自然幅がそれを超える場合は DropWhenNarrow の順で列を落とし
// それでも収まらなければ table.Width + table.Wrap で折り返す(D22)
// MaxWidth 未指定 または自然幅で収まる場合は Width を指定せず自然幅のまま描く
func (t *Table) Render() string {
	headers, rows := t.truncatedData()

	active := make([]int, len(headers))
	for i := range headers {
		active[i] = i
	}

	if t.maxWidth <= 0 {
		ph, pr := projectColumns(headers, rows, active)
		return renderTable(ph, pr, 0, false)
	}

	ph, pr := projectColumns(headers, rows, active)
	rendered := renderTable(ph, pr, 0, false)
	if lipgloss.Width(rendered) <= t.maxWidth {
		return rendered
	}

	for _, dropHeader := range t.dropOrder {
		pos := slices.Index(headers, dropHeader)
		if pos < 0 {
			continue
		}
		if i := slices.Index(active, pos); i >= 0 {
			active = slices.Delete(active, i, i+1)
		}
		if len(active) == 0 {
			break
		}

		ph, pr = projectColumns(headers, rows, active)
		rendered = renderTable(ph, pr, 0, false)
		if lipgloss.Width(rendered) <= t.maxWidth {
			return rendered
		}
	}

	ph, pr = projectColumns(headers, rows, active)
	return renderTable(ph, pr, t.maxWidth, true)
}

// truncatedData は Truncate で指定された列にだけ切り詰めを適用したヘッダーと行のコピーを返す
func (t *Table) truncatedData() ([]string, [][]string) {
	headers := slices.Clone(t.headers)
	rows := make([][]string, len(t.rows))
	for i, row := range t.rows {
		rows[i] = slices.Clone(row)
	}

	for header, maxCells := range t.truncate {
		idx := slices.Index(headers, header)
		if idx < 0 {
			continue
		}
		for _, row := range rows {
			if idx < len(row) {
				row[idx] = ansi.Truncate(row[idx], maxCells, "…")
			}
		}
	}

	return headers, rows
}

// projectColumns は active に含まれる列番号だけを残したヘッダーと行を作る
func projectColumns(headers []string, rows [][]string, active []int) ([]string, [][]string) {
	projHeaders := make([]string, len(active))
	for i, idx := range active {
		projHeaders[i] = headers[idx]
	}

	projRows := make([][]string, len(rows))
	for r, row := range rows {
		projRow := make([]string, len(active))
		for i, idx := range active {
			if idx < len(row) {
				projRow[i] = row[idx]
			}
		}
		projRows[r] = projRow
	}

	return projHeaders, projRows
}

// renderTable は Lip Gloss v2 の table で実際の描画文字列を作る
// width が正の値なら table.Width を指定し wrap が true なら折り返しを有効にする(D22)
func renderTable(headers []string, rows [][]string, width int, wrap bool) string {
	writer := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ColorBorder)).
		Headers(headers...).
		StyleFunc(func(row int, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return tableHeaderStyle
			}
			return tableCellStyle
		})

	for _, row := range rows {
		writer.Row(row...)
	}

	if width > 0 {
		writer = writer.Width(width)
	}
	if wrap {
		writer = writer.Wrap(true)
	}

	return writer.Render()
}
