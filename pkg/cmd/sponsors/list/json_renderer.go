package listcmd

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type sponsorsDTO []struct {
	Login string `json:"login"`
}

type JSONListRenderer struct {
	IO       *iostreams.IOStreams
	Exporter cmdutil.Exporter
}

func (r JSONListRenderer) Render(sponsors Sponsors) error {
	var sponsorsDTO sponsorsDTO
	for _, sponsor := range sponsors {
		sponsorsDTO = append(sponsorsDTO, struct {
			Login string `json:"login"`
		}{Login: string(sponsor.Login)})
	}

	return r.Exporter.Write(r.IO, sponsorsDTO)
}
