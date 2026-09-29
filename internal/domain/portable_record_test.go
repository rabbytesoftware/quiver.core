package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortableRecord_Merge_Description(t *testing.T) {
	testCases := []struct {
		name     string
		record   PortableRecord
		incoming []PortableApp
		want     []PortableApp
	}{
		{
			name:     "empty record plus new apps",
			record:   PortableRecord{},
			incoming: []PortableApp{{Name: "Bruno", Entry: "bruno/.quiver-run"}},
			want:     []PortableApp{{Name: "Bruno", Entry: "bruno/.quiver-run"}},
		},
		{
			name: "replace same entry",
			record: PortableRecord{
				Apps: []PortableApp{{Name: "Bruno", Entry: "bruno/.quiver-run", Icon: "old.png"}},
			},
			incoming: []PortableApp{{Name: "Bruno", Entry: "bruno/.quiver-run", Icon: "new.png"}},
			want:     []PortableApp{{Name: "Bruno", Entry: "bruno/.quiver-run", Icon: "new.png"}},
		},
		{
			name: "keep others",
			record: PortableRecord{
				Apps: []PortableApp{
					{Name: "Bruno", Entry: "bruno/.quiver-run"},
					{Name: "CC Switch", Entry: "cc-switch/.quiver-run"},
				},
			},
			incoming: []PortableApp{{Name: "CC Switch v2", Entry: "cc-switch/.quiver-run"}},
			want: []PortableApp{
				{Name: "Bruno", Entry: "bruno/.quiver-run"},
				{Name: "CC Switch v2", Entry: "cc-switch/.quiver-run"},
			},
		},
		{
			name: "order preserved: kept apps keep position, new apps appended in order",
			record: PortableRecord{
				Apps: []PortableApp{
					{Name: "A", Entry: "a/.quiver-run"},
					{Name: "B", Entry: "b/.quiver-run"},
				},
			},
			incoming: []PortableApp{
				{Name: "C", Entry: "c/.quiver-run"},
				{Name: "B2", Entry: "b/.quiver-run"},
				{Name: "D", Entry: "d/.quiver-run"},
			},
			want: []PortableApp{
				{Name: "A", Entry: "a/.quiver-run"},
				{Name: "B2", Entry: "b/.quiver-run"},
				{Name: "C", Entry: "c/.quiver-run"},
				{Name: "D", Entry: "d/.quiver-run"},
			},
		},
		{
			name: "same name at a new entry supersedes the older build",
			record: PortableRecord{
				Apps: []PortableApp{
					{Name: "Bruno", Entry: "Bruno-1.0/.quiver-run"},
					{Name: "Helper", Entry: "helper"},
				},
			},
			incoming: []PortableApp{{Name: "Bruno", Entry: "Bruno-2.0/.quiver-run"}},
			want: []PortableApp{
				{Name: "Helper", Entry: "helper"},
				{Name: "Bruno", Entry: "Bruno-2.0/.quiver-run"},
			},
		},
		{
			name:     "no incoming apps returns record unchanged",
			record:   PortableRecord{Apps: []PortableApp{{Name: "A", Entry: "a/.quiver-run"}}},
			incoming: nil,
			want:     []PortableApp{{Name: "A", Entry: "a/.quiver-run"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.record.Merge(tc.incoming)
			assert.Equal(t, tc.want, got.Apps)
		})
	}
}

func TestPortableRecord_Merge_DoesNotMutateReceiver(t *testing.T) {
	original := PortableRecord{
		Apps: []PortableApp{{Name: "A", Entry: "a/.quiver-run"}},
	}

	_ = original.Merge([]PortableApp{{Name: "A2", Entry: "a/.quiver-run"}})

	assert.Equal(t, "A", original.Apps[0].Name)
}
