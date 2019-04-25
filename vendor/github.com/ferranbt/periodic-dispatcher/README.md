
# Periodic Dispatcher

Based on [Nomad](https://github.com/hashicorp/nomad) periodic dispatcher.

Launch multiple periodic jobs with a single thread.

```
package main

import (
    "fmt"
    "time"
    
    "github.com/ferranbt/periodic-dispatcher"
)

type Job struct {
    id string
}

func (j *Job) ID() string {
    return j.id
}

func main() {
    dispatcher := periodic.NewDispatcher()
    dispatcher.SetEnabled(true)
    
    dispatcher.Add(&Job{"a"}, 1*time.Second)
    dispatcher.Add(&Job{"b"}, 2*time.Second)
    
    for {
        evnt := <-dispatcher.Events()
        fmt.Println(evnt)
    }
}
```
