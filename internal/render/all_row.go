// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package render

// AllRowID identifies the synthetic first row ("all") that the nodes and namespaces lists add so a whole level can be
// picked at once. It is not a Kubernetes object, so both colorers draw it in the theme color instead of a status color.
const AllRowID = "all"
