import {describe, expect, it} from 'vitest';
import {graphTooltip} from './graph-tooltip';

describe('graph tooltip', () => {
  it('explains a node using only local projection fields', () => {
    expect(graphTooltip({id: 'account', label: 'account', type: 'repository', community: 'platform', path: 'repos/account', summary: 'Account service', connections: 4})).toEqual({title: 'account', type: 'repository', group: 'platform', detail: 'Account service', connections: 4, path: 'repos/account'});
  });
});
