export class VocabularyValidationError extends Error {
  override readonly name = 'VocabularyValidationError';

  constructor(
    readonly concept: string,
    readonly reason: string,
  ) {
    super(`${concept}: ${reason}`);
  }
}
