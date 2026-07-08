/*
Copyright (C) 2023-2026 c1cada

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

For commercial licensing, please contact support@c1cada.dev
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { FlowQuotaDataItem } from '../types'
import {
  buildDashboardFlowData,
  buildFlowFilterOptions,
  buildFlowSankeySpec,
} from './flow'

const rows: FlowQuotaDataItem[] = [
  {
    user_id: 1,
    username: 'alice',
    node_name: 'node-a',
    token_id: 11,
    token_name: 'primary',
    use_group: 'vip',
    channel_id: 101,
    channel_name: 'east',
    model_name: 'gpt-4.1',
    quota: 100,
    token_used: 40,
    count: 2,
  },
  {
    user_id: 1,
    username: 'alice',
    node_name: 'node-a',
    token_id: 11,
    token_name: 'primary',
    use_group: 'vip',
    channel_id: 102,
    channel_name: 'west',
    model_name: 'gpt-4.1',
    quota: 50,
    token_used: 20,
    count: 1,
  },
  {
    user_id: 2,
    username: 'bob',
    node_name: 'node-b',
    token_id: 22,
    token_name: 'backup',
    use_group: 'default',
    channel_id: 101,
    channel_name: 'east',
    model_name: 'claude-4-sonnet',
    quota: 70,
    token_used: 30,
    count: 3,
  },
]

const topLimitRows: FlowQuotaDataItem[] = [
  {
    user_id: 1,
    username: 'alpha',
    use_group: 'vip',
    channel_id: 201,
    channel_name: 'channel-a',
    model_name: 'model-a',
    quota: 100,
    token_used: 1_000,
    count: 1,
  },
  {
    user_id: 2,
    username: 'beta',
    use_group: 'default',
    channel_id: 202,
    channel_name: 'channel-b',
    model_name: 'model-b',
    quota: 80,
    token_used: 10,
    count: 20,
  },
  {
    user_id: 3,
    username: 'gamma',
    use_group: 'free',
    channel_id: 203,
    channel_name: 'channel-c',
    model_name: 'model-c',
    quota: 10,
    token_used: 2_000,
    count: 5,
  },
]

describe('dashboard flow data', () => {
  test('builds normal user token-group-model flow', () => {
    const result = buildDashboardFlowData(rows.slice(0, 2), 'quota', {
      role: 'user',
    })

    assert.equal(result.summary.quota, 150)
    assert.equal(result.summary.tokens, 60)
    assert.equal(result.summary.requests, 3)
    assert.deepEqual(
      result.flow.links.map((link) => [link.source, link.target, link.value]),
      [
        ['group:vip', 'model:gpt-4.1', 150],
        ['token:11', 'group:vip', 150],
      ]
    )
    assert.equal(
      result.flow.nodes.some((node) => node.kind === 'channel'),
      false
    )
  })

  test('builds admin user-group-model-channel flow', () => {
    const result = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
    })

    assert.deepEqual(
      result.flow.links.map((link) => [link.source, link.target, link.value]),
      [
        ['group:default', 'model:claude-4-sonnet', 70],
        ['group:vip', 'model:gpt-4.1', 150],
        ['model:claude-4-sonnet', 'channel:101', 70],
        ['model:gpt-4.1', 'channel:101', 100],
        ['model:gpt-4.1', 'channel:102', 50],
        ['user:1', 'group:vip', 150],
        ['user:2', 'group:default', 70],
      ]
    )
  })

  test('builds root user-node-token-group-model-channel flow', () => {
    const result = buildDashboardFlowData(rows, 'requests', {
      role: 'root',
    })

    assert.deepEqual(
      result.flow.links.map((link) => [link.source, link.target, link.value]),
      [
        ['group:default', 'model:claude-4-sonnet', 3],
        ['group:vip', 'model:gpt-4.1', 3],
        ['model:claude-4-sonnet', 'channel:101', 3],
        ['model:gpt-4.1', 'channel:101', 2],
        ['model:gpt-4.1', 'channel:102', 1],
        ['node:node-a', 'token:11', 3],
        ['node:node-b', 'token:22', 3],
        ['token:11', 'group:vip', 3],
        ['token:22', 'group:default', 3],
        ['user:1', 'node:node-a', 3],
        ['user:2', 'node:node-b', 3],
      ]
    )
  })

  test('filters by selected users and selected nodes', () => {
    const userResult = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      selectedUsers: ['user:2'],
    })
    const nodeResult = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      selectedNodes: [{ kind: 'model', id: 'model:gpt-4.1' }],
    })

    assert.equal(userResult.summary.quota, 70)
    assert.deepEqual(
      userResult.flow.links.map((link) => [
        link.source,
        link.target,
        link.value,
      ]),
      [
        ['group:default', 'model:claude-4-sonnet', 70],
        ['model:claude-4-sonnet', 'channel:101', 70],
        ['user:2', 'group:default', 70],
      ]
    )
    assert.equal(nodeResult.summary.quota, 150)
    assert.deepEqual(
      nodeResult.flow.links.map((link) => [
        link.source,
        link.target,
        link.value,
      ]),
      [
        ['group:vip', 'model:gpt-4.1', 150],
        ['model:gpt-4.1', 'channel:101', 100],
        ['model:gpt-4.1', 'channel:102', 50],
        ['user:1', 'group:vip', 150],
      ]
    )
  })

  test('combines node filters with OR inside a column and AND across columns', () => {
    const sameColumn = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      selectedNodes: [
        { kind: 'model', id: 'model:gpt-4.1' },
        { kind: 'model', id: 'model:claude-4-sonnet' },
      ],
    })
    const crossColumn = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      selectedNodes: [
        { kind: 'model', id: 'model:gpt-4.1' },
        { kind: 'channel', id: 'channel:101' },
      ],
    })

    assert.equal(sameColumn.summary.quota, 220)
    assert.equal(crossColumn.summary.quota, 100)
    assert.deepEqual(
      crossColumn.flow.links.map((link) => [
        link.source,
        link.target,
        link.value,
      ]),
      [
        ['group:vip', 'model:gpt-4.1', 100],
        ['model:gpt-4.1', 'channel:101', 100],
        ['user:1', 'group:vip', 100],
      ]
    )
  })

  test('reconnects links when a middle stage is hidden', () => {
    const result = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      visibleStages: ['user', 'model', 'channel'],
    })

    assert.deepEqual(
      result.flow.links.map((link) => [link.source, link.target, link.value]),
      [
        ['model:claude-4-sonnet', 'channel:101', 70],
        ['model:gpt-4.1', 'channel:101', 100],
        ['model:gpt-4.1', 'channel:102', 50],
        ['user:1', 'model:gpt-4.1', 150],
        ['user:2', 'model:claude-4-sonnet', 70],
      ]
    )
    assert.equal(
      result.flow.nodes.some((node) => node.kind === 'group'),
      false
    )
  })

  test('builds user and node filter options with stable values', () => {
    const options = buildFlowFilterOptions(rows, 'quota')
    const result = buildDashboardFlowData(topLimitRows, 'quota', {
      role: 'admin',
      topNodeLimit: 1,
      overflowMode: 'aggregate',
    })

    assert.deepEqual(
      options.users.map((user) => [user.value, user.label, user.valueLabel]),
      [
        ['user:1', 'alice', '150'],
        ['user:2', 'bob', '70'],
      ]
    )
    assert.deepEqual(
      result.filterOptions.nodes
        .filter((option) => option.kind === 'model')
        .map((option) => [option.value, option.valueLabel]),
      [
        ['model:model-a', '100'],
        ['model:model-b', '80'],
        ['model:model-c', '10'],
      ]
    )
  })

  test('aggregates or hides overflow paths according to overflow mode', () => {
    const aggregated = buildDashboardFlowData(topLimitRows, 'quota', {
      role: 'admin',
      topNodeLimit: 2,
      overflowMode: 'aggregate',
      otherNodeLabel: (kind) => `Other ${kind}`,
    })
    const hidden = buildDashboardFlowData(topLimitRows, 'quota', {
      role: 'admin',
      topNodeLimit: 2,
      overflowMode: 'hide',
      otherNodeLabel: (kind) => `Other ${kind}`,
    })

    const aggregatedNodeIds = new Set(
      aggregated.flow.nodes.map((node) => node.id)
    )
    const hiddenNodeIds = new Set(hidden.flow.nodes.map((node) => node.id))
    const aggregatedFirstStepTotal = aggregated.flow.links
      .filter((link) => link.source.startsWith('user:'))
      .reduce((sum, link) => sum + link.value, 0)
    const hiddenFirstStepTotal = hidden.flow.links
      .filter((link) => link.source.startsWith('user:'))
      .reduce((sum, link) => sum + link.value, 0)

    assert.equal(aggregated.summary.quota, 190)
    assert.equal(hidden.summary.quota, 190)
    assert.equal(aggregatedFirstStepTotal, 190)
    assert.equal(hiddenFirstStepTotal, 180)
    assert.equal(aggregatedNodeIds.has('user:__other__'), true)
    assert.equal(hiddenNodeIds.has('user:__other__'), false)
  })

  test('ranks top nodes using the selected flow metric', () => {
    const byQuota = buildDashboardFlowData(topLimitRows, 'quota', {
      role: 'admin',
      topNodeLimit: 1,
      overflowMode: 'aggregate',
    })
    const byRequests = buildDashboardFlowData(topLimitRows, 'requests', {
      role: 'admin',
      topNodeLimit: 1,
      overflowMode: 'aggregate',
    })
    const byTokens = buildDashboardFlowData(topLimitRows, 'tokens', {
      role: 'admin',
      topNodeLimit: 1,
      overflowMode: 'aggregate',
    })

    assert.equal(
      byQuota.flow.nodes.some((node) => node.id === 'user:1'),
      true
    )
    assert.equal(
      byRequests.flow.nodes.some((node) => node.id === 'user:2'),
      true
    )
    assert.equal(
      byTokens.flow.nodes.some((node) => node.id === 'user:3'),
      true
    )
  })

  test('highlights full paths that contain an active node or active link', () => {
    const byNode = buildDashboardFlowData(rows, 'quota', {
      role: 'root',
      activeNode: { kind: 'user', id: 'user:1' },
    })
    const byLink = buildDashboardFlowData(rows, 'quota', {
      role: 'root',
      activeLink: { source: 'model:gpt-4.1', target: 'channel:101' },
    })

    const nodeState = new Map(
      byNode.flow.nodes.map((node) => [
        node.id,
        { highlighted: node.highlighted, dimmed: node.dimmed },
      ])
    )
    const linkState = new Map(
      byLink.flow.links.map((link) => [
        `${link.source}->${link.target}`,
        { highlighted: link.highlighted, dimmed: link.dimmed },
      ])
    )

    assert.deepEqual(nodeState.get('user:1'), {
      highlighted: true,
      dimmed: false,
    })
    assert.deepEqual(nodeState.get('user:2'), {
      highlighted: false,
      dimmed: true,
    })
    assert.deepEqual(linkState.get('model:gpt-4.1->channel:101'), {
      highlighted: true,
      dimmed: false,
    })
    assert.deepEqual(linkState.get('model:gpt-4.1->channel:102'), {
      highlighted: false,
      dimmed: true,
    })
  })

  test('builds Sankey spec with quota token request tooltips', () => {
    const result = buildDashboardFlowData(rows.slice(0, 1), 'quota', {
      role: 'root',
    })
    const flowSpec = buildFlowSankeySpec(result.flow, 'Flow')
    const values = flowSpec.data[0].values[0]
    const aliceNode = values.nodes.find(
      (node: Record<string, unknown>) => node.key === 'user:1'
    )
    const userNodeLink = values.links.find(
      (link: Record<string, unknown>) =>
        link.source === 'user:1' && link.target === 'node:node-a'
    )

    assert.equal(flowSpec.type, 'sankey')
    assert.equal(flowSpec.title.text, 'Flow')
    assert.deepEqual(flowSpec.emphasis, { enable: false })
    assert.equal(flowSpec.tooltip.mark.visible(aliceNode), true)
    assert.equal(flowSpec.tooltip.mark.visible(userNodeLink), true)
    assert.equal(flowSpec.animation, false)
    assert.equal(values.nodes.length, 6)
    assert.equal(values.links.length, 5)
    assert.equal(aliceNode.name, 'alice')
    assert.match(userNodeLink.linkColor, /^rgba\(/)

    const tooltipRows = flowSpec.tooltip.mark.content
    assert.deepEqual(
      tooltipRows
        .filter((row: Record<string, unknown>) =>
          typeof row.visible === 'function' ? row.visible(userNodeLink) : true
        )
        .map((row: Record<string, unknown>) => [
          row.key,
          typeof row.value === 'function' ? row.value(userNodeLink) : row.value,
        ]),
      [
        ['Quota', '100'],
        ['Tokens', '40'],
        ['Requests', '2'],
        ['Share', '100.0%'],
      ]
    )
  })
})
