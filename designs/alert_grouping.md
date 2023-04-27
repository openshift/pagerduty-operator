# Grouping alerts from a cluster

Author: achvatal

Last updated: 26 Apr

## Goals

Limit the number of alerts from clusters so that a single root problem on a cluster should only generate a single alert.

## Current state

Every critical alert generates a Pager Duty incident, regardless of timing

## Proposal

Enable time-based alert grouping on Pager Duty services to automatically merge new incidents into an existing incident (for the same service) if its most recent alert was within a configurable timeframe (defaulting to one hour).

This can be configured by adding an `AlertGroupingParameters` parameter to the Service type where it's created in `pkg/pagerduty/service.go`, this would affect new clusters. For existing clusters, a check could be added to the pagerdutyintegration reconcile loop that ensures this setting is configured or a standalone tool cool be created to loop through all existing Pager Duty services and configure them.
