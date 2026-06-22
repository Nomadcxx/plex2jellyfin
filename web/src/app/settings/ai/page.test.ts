import { describe, expect, it } from 'vitest';
import { buildAISettingsPayload, coerceAINumberInput } from './settingsPayload';

describe('AI settings payload safety', () => {
  it('does not coerce blank numeric fields to zero', () => {
    expect(coerceAINumberInput('', { min: 0, max: 1 })).toBeUndefined();
  });

  it('clamps numeric fields to the configured safe range', () => {
    expect(coerceAINumberInput('-0.25', { min: 0, max: 1 })).toBe(0);
    expect(coerceAINumberInput('1.25', { min: 0, max: 1 })).toBe(1);
    expect(coerceAINumberInput('0.72', { min: 0, max: 1 })).toBe(0.72);
  });

  it('builds a payload that preserves selected models and omits invalid numeric edits', () => {
    const payload = buildAISettingsPayload({
      enabled: true,
      model: 'minimax-2.5',
      fallback_model: 'llama3.1:8b',
      confidence_threshold: '',
      auto_trigger_threshold: '0.7',
      timeout_seconds: '0',
      max_retries: 'abc',
    });

    expect(payload).toMatchObject({
      enabled: true,
      model: 'minimax-2.5',
      fallback_model: 'llama3.1:8b',
      auto_trigger_threshold: 0.7,
      timeout_seconds: 1,
    });
    expect(payload).not.toHaveProperty('confidence_threshold');
    expect(payload).not.toHaveProperty('max_retries');
  });
});
