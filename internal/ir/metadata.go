package ir

const MetadataCacheKey = "node_metadata"

type Metadata struct {
	Embeds map[NodeID]*EmbedInfo
	Assets map[NodeID]*AssetInfo
}

func NewMetadata() *Metadata {
	return &Metadata{
		Embeds: map[NodeID]*EmbedInfo{},
		Assets: map[NodeID]*AssetInfo{},
	}
}

func GetMetadata(cache map[string]any) *Metadata {
	if cache == nil {
		return nil
	}

	if raw, ok := cache[MetadataCacheKey]; ok {
		if m, ok := raw.(*Metadata); ok {
			return m
		}
	}

	return nil
}

func EnsureMetadata(cache map[string]any) *Metadata {
	if cache == nil {
		return NewMetadata()
	}

	if m := GetMetadata(cache); m != nil {
		return m
	}

	m := NewMetadata()
	cache[MetadataCacheKey] = m
	return m
}

func (m *Metadata) EmbedFor(n Node) *EmbedInfo {
	if m == nil || n == nil {
		return nil
	}

	return m.Embeds[n.ID()]
}

func (m *Metadata) AssetFor(n Node) *AssetInfo {
	if m == nil || n == nil {
		return nil
	}

	return m.Assets[n.ID()]
}
