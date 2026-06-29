import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';

export function trimAndValidateIdentifier(concept: string, value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new VocabularyValidationError(
      concept,
      'identifier cannot be empty or whitespace',
    );
  }
  return trimmed;
}
