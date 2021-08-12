package extension

import "os"

func makeSymlink(oldname, newname string) error {
	// Create a regular file that contains the location of the directory where to find this extension. We
	// avoid relying on symlinks because creating them on Windows requires administrator privileges.
	f, err := os.OpenFile(newname, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(oldname)
	return err
}
