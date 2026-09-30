import {describe, expect, it} from 'vitest';
import {localDevBackendOrigin} from '../vite.config';

describe('localDevBackendOrigin', () => {
  it('accepts a loopback HTTP origin', () => {
    expect(localDevBackendOrigin('http://127.0.0.1:43123')).toBe('http://127.0.0.1:43123');
  });

  it.each(['https://127.0.0.1:43123', 'http://example.test', 'http://127.0.0.1:43123/api', 'http://127.0.0.1:43123/?x=1'])('rejects unsafe proxy target %s', value => {
    expect(() => localDevBackendOrigin(value)).toThrow('AIOS_UI_ORIGIN');
  });
});
