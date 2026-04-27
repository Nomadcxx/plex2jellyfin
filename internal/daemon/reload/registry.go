package reload

var Default = NewSupervisor()

func Register(r Reloadable) {
	Default.Register(r)
}
