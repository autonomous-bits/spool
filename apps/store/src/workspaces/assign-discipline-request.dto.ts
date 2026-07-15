import { BadRequestException } from '@nestjs/common';
import {
  isDiscipline,
  type Discipline,
} from '../domain/types/vocabulary/discipline.js';

export interface AssignDisciplineRequest {
  discipline: Discipline;
}

export function parseAssignDisciplineRequest(body: unknown): AssignDisciplineRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;
  const discipline = record.discipline;
  if (!isDiscipline(discipline)) {
    throw new BadRequestException(
      'discipline must be one of: product, architecture, design, engineering, security, governance',
    );
  }

  return { discipline } satisfies AssignDisciplineRequest;
}

export function parseDisciplineParam(value: unknown): Discipline {
  if (!isDiscipline(value)) {
    throw new BadRequestException(
      'discipline must be one of: product, architecture, design, engineering, security, governance',
    );
  }

  return value;
}
