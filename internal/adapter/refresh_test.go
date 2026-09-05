package adapter

import (
	"slices"
	"testing"
)

func TestIndexRefresher_PlanRefresh_argv(t *testing.T) {
	tests := []struct {
		mgr  Adapter
		want []string
	}{
		{Brew{}, []string{"brew", "update"}},
		{Linuxbrew{}, []string{"brew", "update"}},
		{Apt{}, []string{"sudo", "apt-get", "update"}},
		{Pacman{}, []string{"sudo", "pacman", "-Sy", "--noconfirm"}},
		{Paru{}, []string{"paru", "-Sy", "--noconfirm"}},
		{Yay{}, []string{"yay", "-Sy", "--noconfirm"}},
		{Dnf{}, []string{"sudo", "dnf", "makecache"}},
		{Apk{}, []string{"sudo", "apk", "update"}},
		{Scoop{}, []string{"scoop", "update"}},
		{Winget{}, []string{"winget", "source", "update"}},
	}
	for _, tc := range tests {
		t.Run(tc.mgr.Name(), func(t *testing.T) {
			refresher, ok := tc.mgr.(IndexRefresher)
			if !ok {
				t.Fatalf("%s does not implement IndexRefresher", tc.mgr.Name())
			}
			got := refresher.PlanRefresh()
			if !slices.Equal(got, tc.want) {
				t.Fatalf("PlanRefresh() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIndexRefresher_liveRegistryManagersOmitRefresh(t *testing.T) {
	// Live registries already talk to the remote on ListOutdated. A separate
	// index fetch would be wasted work and is not in the issue table.
	live := []Adapter{
		Mas{}, Bun{}, Npm{}, Pnpm{}, Yarn{}, Uv{}, Pipx{}, PipUser{}, Cargo{}, Volta{},
		Choco{}, Snap{}, Vscode{},
	}
	for _, mgr := range live {
		t.Run(mgr.Name(), func(t *testing.T) {
			if _, ok := mgr.(IndexRefresher); ok {
				t.Fatalf("%s implements IndexRefresher; live-registry managers must omit it", mgr.Name())
			}
		})
	}
}

func TestPacman_PlanRefresh_isSyncOnly(t *testing.T) {
	// -Syu is the v4.3.0 OS vendor step. Tracked refresh must stay -Sy.
	got := Pacman{}.PlanRefresh()
	if slices.Contains(got, "-Syu") {
		t.Fatalf("PlanRefresh = %v, must not use -Syu", got)
	}
	if !slices.Contains(got, "-Sy") {
		t.Fatalf("PlanRefresh = %v, want -Sy", got)
	}
}
