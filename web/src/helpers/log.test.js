/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import assert from 'node:assert/strict';
import test from 'node:test';
import { getRetryPathText } from './log.js';

test('getRetryPathText prefers retry_path_text', () => {
  assert.equal(
    getRetryPathText({ retry_path_text: '11 -> 18 -> 24' }, 11),
    '11 -> 18 -> 24',
  );
});

test('getRetryPathText falls back to admin_info.use_channel', () => {
  assert.equal(
    getRetryPathText({ admin_info: { use_channel: [11, 18, 24] } }, 11),
    '11 -> 18 -> 24',
  );
});

test('getRetryPathText falls back to default channel when no retry metadata exists', () => {
  assert.equal(getRetryPathText({}, 11), '11');
});
