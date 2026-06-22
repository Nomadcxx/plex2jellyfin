export type NumberBounds = {
  min?: number;
  max?: number;
};

const AI_NUMBER_FIELDS: Record<string, NumberBounds> = {
  confidence_threshold: { min: 0, max: 1 },
  auto_trigger_threshold: { min: 0, max: 1 },
  timeout_seconds: { min: 1, max: 600 },
  max_retries: { min: 0, max: 20 },
  hourly_limit: { min: 0 },
  daily_limit: { min: 0 },
  enhancement_interval_seconds: { min: 1 },
};

export function coerceAINumberInput(value: unknown, bounds: NumberBounds = {}): number | undefined {
  if (value === '' || value === null || value === undefined) return undefined;

  const num = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(num)) return undefined;

  let next = num;
  if (typeof bounds.min === 'number') next = Math.max(bounds.min, next);
  if (typeof bounds.max === 'number') next = Math.min(bounds.max, next);
  return next;
}

export function buildAISettingsPayload(draft: Record<string, unknown>): Record<string, unknown> {
  const payload: Record<string, unknown> = { ...draft };
  for (const [key, bounds] of Object.entries(AI_NUMBER_FIELDS)) {
    const coerced = coerceAINumberInput(draft[key], bounds);
    if (coerced === undefined) delete payload[key];
    else payload[key] = coerced;
  }
  return payload;
}
