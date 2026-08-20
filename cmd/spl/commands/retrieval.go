package commands

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

type retrievalFilterFlags struct {
	labels        []string
	textEquals    []string
	numberEquals  []string
	numberMinimum []string
	numberMaximum []string
}

func (flags *retrievalFilterFlags) add(command *cobra.Command) {
	command.Flags().StringArrayVar(&flags.labels, "label", nil, "required node label (repeatable)")
	command.Flags().StringArrayVar(&flags.textEquals, "property-text", nil, "indexed text property equality as key=value (repeatable)")
	command.Flags().StringArrayVar(&flags.numberEquals, "property-number", nil, "indexed numeric property equality as key=value (repeatable)")
	command.Flags().StringArrayVar(&flags.numberMinimum, "property-min", nil, "indexed numeric property lower bound as key=value (repeatable)")
	command.Flags().StringArrayVar(&flags.numberMaximum, "property-max", nil, "indexed numeric property upper bound as key=value (repeatable)")
}

func (flags retrievalFilterFlags) predicates() ([]repository.MetadataPredicate, error) {
	type predicateValues struct {
		text, number, minimum, maximum *string
	}
	values := make(map[string]*predicateValues)
	add := func(items []string, field func(*predicateValues) **string) error {
		for _, item := range items {
			key, value, ok := strings.Cut(item, "=")
			if !ok || key == "" {
				return fmt.Errorf("property filter %q must be key=value", item)
			}
			entry := values[key]
			if entry == nil {
				entry = &predicateValues{}
				values[key] = entry
			}
			target := field(entry)
			if *target != nil {
				return fmt.Errorf("property filter for %q is repeated", key)
			}
			valueCopy := value
			*target = &valueCopy
		}
		return nil
	}
	if err := add(flags.textEquals, func(value *predicateValues) **string { return &value.text }); err != nil {
		return nil, err
	}
	if err := add(flags.numberEquals, func(value *predicateValues) **string { return &value.number }); err != nil {
		return nil, err
	}
	if err := add(flags.numberMinimum, func(value *predicateValues) **string { return &value.minimum }); err != nil {
		return nil, err
	}
	if err := add(flags.numberMaximum, func(value *predicateValues) **string { return &value.maximum }); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	predicates := make([]repository.MetadataPredicate, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		if value.text != nil && (value.number != nil || value.minimum != nil || value.maximum != nil) ||
			value.number != nil && (value.minimum != nil || value.maximum != nil) {
			return nil, fmt.Errorf("property filter for %q mixes incompatible comparisons", key)
		}
		predicate := repository.MetadataPredicate{Key: key}
		if value.text != nil {
			predicate.TextEquals = value.text
		} else if value.number != nil {
			number, err := parseFilterNumber(key, *value.number)
			if err != nil {
				return nil, err
			}
			predicate.NumberEquals = &number
		} else {
			if value.minimum != nil {
				number, err := parseFilterNumber(key, *value.minimum)
				if err != nil {
					return nil, err
				}
				predicate.NumberMin = &number
			}
			if value.maximum != nil {
				number, err := parseFilterNumber(key, *value.maximum)
				if err != nil {
					return nil, err
				}
				predicate.NumberMax = &number
			}
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}

func parseFilterNumber(key, raw string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("numeric property filter %q has invalid value %q", key, raw)
	}
	return value, nil
}

func NewFilterCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, token string
	var filters retrievalFilterFlags
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use: "filter", Short: "Filter nodes by labels and indexed properties",
		Long:    "Return JSON nodes selected by labels and typed indexed-property filters. SQL and projection query syntax are not accepted.",
		Example: "  spl filter --branch main --label Task --property-text status=open\n  spl filter --branch main --property-min priority=3",
		Args:    cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			predicates, err := filters.predicates()
			if err != nil {
				return err
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGFilter(command.Context(), resolve.FilterRequest{
				Selector: snapshotSelectorFlag(command, "commit", branch, commit), Labels: filters.labels, Predicates: predicates,
				ContinuationToken: token, Budget: budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			return writeJSON(command, result, "filter")
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch-head projection to query")
	command.Flags().StringVar(&commit, "commit", "", "commit selector; only the current branch head is supported")
	filters.add(command)
	budgetFlags.addPagedQueryFlags(command, &token)
	_ = command.MarkFlagRequired("branch")
	return command
}

func NewSearchCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, query, token string
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use: "search", Short: "Search nodes lexically",
		Long:    "Return JSON lexical matches from the branch-head projection.",
		Example: "  spl search --branch main --query incident",
		Args:    cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGSearch(command.Context(), resolve.SearchRequest{
				Selector: snapshotSelectorFlag(command, "commit", branch, commit), Query: query,
				ContinuationToken: token, Budget: budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			return writeJSON(command, result, "search")
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch-head projection to query")
	command.Flags().StringVar(&commit, "commit", "", "commit selector; only the current branch head is supported")
	command.Flags().StringVar(&query, "query", "", "lexical query")
	budgetFlags.addPagedQueryFlags(command, &token)
	_ = command.MarkFlagRequired("branch")
	_ = command.MarkFlagRequired("query")
	return command
}

func NewSearchExpandCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	return newContextualCommand("search-expand", "Search nodes and expand graph context", "Search lexical or typed-filter evidence, then return bounded graph context as JSON.", "spl search-expand --branch main --query incident --direction out --edge-type RELATES_TO", toolProvider, false)
}

func NewContextCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	return newContextualCommand("context", "Assemble evidence-focused graph context", "Return JSON evidence and bounded graph context from a lexical query or typed filters.", "spl context --branch main --label Task --property-text status=open --direction both", toolProvider, true)
}

func newContextualCommand(use, short, long, example string, toolProvider func() (*resolve.ResolveTool, error), evidenceFirst bool) *cobra.Command {
	var branch, commit, query, direction string
	var edgeTypes []string
	var seedLimit int
	var filters retrievalFilterFlags
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use: use, Short: short, Long: long, Example: "  " + example,
		Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			predicates, err := filters.predicates()
			if err != nil {
				return err
			}
			if err := validateContextualSeedSelector(query, filters.labels, predicates); err != nil {
				return err
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			request := resolve.SearchExpandRequest{
				Selector:  snapshotSelectorFlag(command, "commit", branch, commit),
				Seeds:     resolve.SeedSelector{Query: query, Labels: filters.labels, Predicates: predicates},
				SeedLimit: seedLimit, Direction: resolve.Direction(direction), EdgeTypes: edgeTypes,
				Budget: budgetFlags.request(command),
			}
			if evidenceFirst {
				result, err := tool.EDGContext(command.Context(), request)
				if err != nil {
					return err
				}
				return writeJSON(command, result, "context")
			}
			result, err := tool.EDGSearchExpand(command.Context(), request)
			if err != nil {
				return err
			}
			return writeJSON(command, result, "search-expand")
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch-head projection to query")
	command.Flags().StringVar(&commit, "commit", "", "commit selector; only the current branch head is supported")
	command.Flags().StringVar(&query, "query", "", "lexical query (exclusive with filter flags)")
	command.Flags().StringVar(&direction, "direction", string(resolve.DirectionOut), "edge direction: out, in, or both")
	command.Flags().StringArrayVar(&edgeTypes, "edge-type", nil, "edge type to traverse (repeatable)")
	command.Flags().IntVar(&seedLimit, "seed-limit", 0, "maximum evidence seeds before expansion")
	filters.add(command)
	budgetFlags.addReadBudgetFlags(command)
	budgetFlags.addTraversalBudgetFlags(command)
	_ = command.MarkFlagRequired("branch")
	return command
}

func validateContextualSeedSelector(query string, labels []string, predicates []repository.MetadataPredicate) error {
	hasQuery := strings.TrimSpace(query) != ""
	hasFilters := len(labels) > 0 || len(predicates) > 0
	if !hasQuery && !hasFilters {
		return fmt.Errorf("provide --query or at least one typed filter")
	}
	if hasQuery && hasFilters {
		return fmt.Errorf("--query cannot be combined with typed filter flags")
	}
	return nil
}

func writeJSON(command *cobra.Command, result any, operation string) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err := command.OutOrStdout().Write(data); err != nil {
		return fmt.Errorf("write %s result: %w", operation, err)
	}
	return nil
}
