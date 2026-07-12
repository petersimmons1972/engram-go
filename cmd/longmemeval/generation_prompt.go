package main

import "github.com/petersimmons1972/engram/internal/longmemeval"

// selectGenerationPrompt builds the generation prompt shared by normal recall
// and atom-oracle runs. Variant precedence is significant: temporal augmentation
// wins over date injection, followed by the remaining prompt levers. H12 is
// applied after variant selection so it prefixes rather than replaces the prompt.
func selectGenerationPrompt(cfg *Config, runOpts longmemeval.RunOpts, item longmemeval.Item, contextBlocks []string) string {
	var prompt string
	switch {
	case cfg.TemporalPromptAug:
		prompt = longmemeval.GenerationPromptForTypeWithTemporalAug(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.InjectQuestionDate:
		prompt = longmemeval.GenerationPromptForTypeWithDateInjection(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.PreferenceEnumerate:
		prompt = longmemeval.GenerationPromptForTypePreferenceEnumerate(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.PreferenceGround:
		prompt = longmemeval.GenerationPromptForTypePreferenceGround(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.AntiHedgePrompts:
		prompt = longmemeval.GenerationPromptForTypeAntiHedge(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.KURecencyPrompt:
		prompt = longmemeval.GenerationPromptForTypeKURecency(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	default:
		prompt = longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, contextBlocks)
	}
	return runOpts.ApplyEnumerateFirst(prompt, item.Question, item.QuestionType)
}
