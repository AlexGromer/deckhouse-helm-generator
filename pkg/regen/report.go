package regen

import (
	"fmt"
	"strings"
)

func (p *Plan) Conflicts() int {
	c := 0
	for _, f := range p.Files {
		if f.Action == ActionConflict || f.Unresolved {
			c++
		}
	}
	return c
}

func (p *Plan) Summary() string {
	counts := make(map[FileAction]int)
	for _, f := range p.Files {
		counts[f.Action]++
	}

	var parts []string
	if c := counts[ActionUpdated]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", c))
	}
	if c := counts[ActionAdded]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d added", c))
	}
	if c := counts[ActionRemoved]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", c))
	}
	if c := counts[ActionMerged]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d merged", c))
	}
	if c := counts[ActionKept]; c > 0 {
		parts = append(parts, fmt.Sprintf("%d kept", c))
	}

	total := len(p.Files) - counts[ActionUnchanged]
	if total == 0 {
		return "No changes"
	}

	summary := fmt.Sprintf("Merged %d files: %s", total, strings.Join(parts, ", "))
	if c := p.Conflicts(); c > 0 {
		summary += fmt.Sprintf("; %d conflict -> MERGE.md", c)
	}

	return summary
}

func (p *Plan) MergeReport() string {
	var sb strings.Builder
	sb.WriteString("<!-- dhg merge report -->\n")
	sb.WriteString("# Повторная генерация: конфликты\n\n")
	sb.WriteString("Разрешите конфликты и закоммитьте; следующий `dhg generate --update` проверит, что маркеров не осталось.\n\n")

	var yamlConflicts []string
	var textConflicts []string
	var removedConflicts []string

	for _, f := range p.Files {
		if f.Action == ActionConflict || f.Unresolved {
			if f.Conflicts == 1 && f.Base != nil && ((f.Ours == nil && f.Theirs != nil) || (f.Ours != nil && f.Theirs == nil)) { // Modify/delete
				s := fmt.Sprintf("| %s | ", f.Path)
				if f.Ours == nil {
					s += "удалён | изменил |"
				} else {
					s += "изменил | удалён |"
				}
				s += fmt.Sprintf(" .dhg/base/%s.base |", f.Path)
				removedConflicts = append(removedConflicts, s)
			} else if isStructuralYAML(f.Path) {
				yamlConflicts = append(yamlConflicts, fmt.Sprintf("| %s | %d |", f.Path, f.Conflicts))
			} else {
				hunks := f.Conflicts
				if hunks == 0 {
					hunks = 1
				}
				textConflicts = append(textConflicts, fmt.Sprintf("- %s — %d участок", f.Path, hunks))
			}
		}
	}

	if len(yamlConflicts) > 0 {
		sb.WriteString("## Значения YAML\n\n")
		sb.WriteString("| Файл | Конфликты |\n|---|---|\n")
		for _, c := range yamlConflicts {
			sb.WriteString(c + "\n")
		}
		sb.WriteString("\n")
	}

	if len(textConflicts) > 0 {
		sb.WriteString("## Текстовые конфликты (маркеры в файле)\n\n")
		for _, c := range textConflicts {
			sb.WriteString(c + "\n")
		}
		sb.WriteString("\n")
	}

	if len(removedConflicts) > 0 {
		sb.WriteString("## Изменено с одной стороны, удалено с другой\n\n")
		sb.WriteString("| Файл | Ваше | Генератор | Где новая версия |\n|---|---|---|---|\n")
		for _, c := range removedConflicts {
			sb.WriteString(c + "\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
