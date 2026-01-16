package quarkgl

// Material is a minimal surface description.
type Material struct {
	BaseColor Color
	Opacity   uint8 // 0..255. 255 means opaque.
}

// LightMode defines minimal lighting options.
type LightMode uint8

const (
	LightOff LightMode = iota
	LightAmbientDirectional
)

// Light is a minimal light setup.
type Light struct {
	Mode      LightMode
	Ambient   Scalar // 0..1
	Dir       Vec3   // direction *towards* the scene
	DirAmount Scalar // 0..1
}

// CameraType selects camera projection.
type CameraType uint8

const (
	CameraPerspective CameraType = iota
	CameraOrtho
)

// Camera describes the viewing transform.
type Camera struct {
	Type CameraType

	Position Vec3
	Target   Vec3
	Up       Vec3

	// Perspective.
	FOVYRad Scalar

	// Orthographic (half-height).
	OrthoSize Scalar

	Near Scalar
	Far  Scalar
}

// View returns the camera view matrix.
func (c Camera) View() Mat4 {
	up := c.Up
	if up == (Vec3{}) {
		up = V3(0, 1, 0)
	}
	return Mat4LookAt(c.Position, c.Target, up)
}

// Projection returns the projection matrix for a target aspect.
func (c Camera) Projection(aspect Scalar) Mat4 {
	switch c.Type {
	case CameraOrtho:
		size := c.OrthoSize
		if size == 0 {
			size = 1
		}
		top := size
		bottom := -size
		right := size * aspect
		left := -right
		return Mat4Ortho(left, right, bottom, top, c.Near, c.Far)
	default:
		fov := c.FOVYRad
		if fov == 0 {
			fov = Scalar(1.0)
		}
		return Mat4Perspective(fov, aspect, c.Near, c.Far)
	}
}

// Vertex is a mesh vertex.
type Vertex struct {
	Pos    Vec3
	Normal Vec3
	Color  Color
}

// Mesh is a triangle mesh with an object transform.
type Mesh struct {
	Enabled bool

	Vertices []Vertex
	Indices  []uint16 // triangle list

	Transform Mat4
	Material  Material
}

// Scene is a collection of objects to render.
type Scene struct {
	Camera Camera
	Light  Light

	meshes []Mesh
	alive  []bool
}

// CreateScene allocates a scene with a fixed mesh capacity.
func CreateScene(maxMeshes int) *Scene {
	if maxMeshes < 0 {
		maxMeshes = 0
	}
	return &Scene{
		Camera: Camera{
			Type:      CameraPerspective,
			Position:  V3(0, 0, 3),
			Target:    V3(0, 0, 0),
			Up:        V3(0, 1, 0),
			FOVYRad:   Scalar(1.0),
			Near:      Scalar(0.05),
			Far:       Scalar(100),
			OrthoSize: Scalar(1),
		},
		Light: Light{
			Mode:      LightAmbientDirectional,
			Ambient:   Scalar(0.25),
			Dir:       Normalize(V3(1, 1, 1)),
			DirAmount: Scalar(0.75),
		},
		meshes: make([]Mesh, maxMeshes),
		alive:  make([]bool, maxMeshes),
	}
}

// AddMesh adds a mesh to the scene and returns its id or -1 if full.
func (s *Scene) AddMesh(m Mesh) int {
	if s == nil {
		return -1
	}
	for i := range s.meshes {
		if s.alive[i] {
			continue
		}
		if m.Transform == (Mat4{}) {
			m.Transform = Mat4Identity()
		}
		if m.Material.Opacity == 0 {
			m.Material.Opacity = 0xFF
		}
		if m.Material.BaseColor == (Color{}) {
			m.Material.BaseColor = RGB(0xCC, 0xCC, 0xCC)
		}
		m.Enabled = true
		s.meshes[i] = m
		s.alive[i] = true
		return i
	}
	return -1
}

// RemoveMesh removes a mesh by id.
func (s *Scene) RemoveMesh(id int) {
	if s == nil || id < 0 || id >= len(s.meshes) {
		return
	}
	s.alive[id] = false
	s.meshes[id] = Mesh{}
}

// SetMeshEnabled enables/disables a mesh by id.
func (s *Scene) SetMeshEnabled(id int, enabled bool) {
	if s == nil || id < 0 || id >= len(s.meshes) || !s.alive[id] {
		return
	}
	s.meshes[id].Enabled = enabled
}

// UpdateMeshTransform updates a mesh transform by id.
func (s *Scene) UpdateMeshTransform(id int, m Mat4) {
	if s == nil || id < 0 || id >= len(s.meshes) || !s.alive[id] {
		return
	}
	s.meshes[id].Transform = m
}

func (s *Scene) eachMesh(fn func(m *Mesh)) {
	for i := range s.meshes {
		if !s.alive[i] {
			continue
		}
		fn(&s.meshes[i])
	}
}
