# better-groupcache

[![Go Report Card](https://goreportcard.com/badge/github.com/Hawk-Zhou/better-groupcache)](https://goreportcard.com/report/github.com/Hawk-Zhou/better-groupcache)
[![codecov](https://codecov.io/gh/Hawk-Zhou/better-groupcache/branch/main/graph/badge.svg?token=ASAM33FCEM)](https://codecov.io/gh/Hawk-Zhou/better-groupcache)  

## What is This

This is my imitation of [groupcache](https://github.com/golang/groupcache), following the instruction of a tutorial, [geecache](https://geektutu.com/post/geecache.html), that teaches how to implement groupcache. My implementation includes improvements (perhaps) on [geecache](https://geektutu.com/post/geecache.html) and [groupcache](https://github.com/golang/groupcache). The purpose of this project is to practice go programming. 

## For the Record

Before boasting about my improvements, for honesty, I have to acknowledge some facts.  

I am grateful for the tutorial and groupcache. I learnt a lot from them. My claimed improvements are just based on my speculation and are conjectures. I tried to give my analysis and reasons. Consider my improvements as a student's attempt to do a homework.  

This is not a 100% implementation of groupcache. For example, it lacks statistics collection function.  

I borrowed one test (named gee_test) from geecache to supplement my tests for lru-k. There are also other code segments that may bear similarity with geecache because I followed the geecache tutorial and referred to it a lot during my coding process. In no ways I am advertising the originality of such segments. No line in the repo is written with no knowledge of what it is doing. I may forget what it is doing later but right now I know clearly how it works. 

To elaborate again and conclude, this is just an imitative implementation, with modification and improvements, that aims to facilitate my go programming skills.  

## Major Improvements

### LRU-2

> This is suggested in the comments below the day1 tutorial by [liyu4](https://github.com/liyu4). 

The underlying cache method used by the two previous cache projects is **LRU**. A problem with LRU is that sporadic read/write could evict frequently accessed data because only one access is needed to load the data into cache (and evicts other data). **LRU-K** , which require K accesses to accommodate the data into cache, is a solution to the problem. I wrapped the original LRU with an FIFO queue that **implements LRU-2** while keeping the interfaces the same.

### More robust singleflight

> For better or worse, this is original.

Both geecache and groupcache implements *singleflight* that holds all but one calls to a function with the same key to uniquely identify these calls. After the one allowed call returns, the held calls all return and share the same return values with the allowed one.

The problem with the implementation is that, panicked calls are not handled properly. As shown below, if the call panics, the held calls will never end waiting because the panicked call exits before it calls `c.wg.Done()`. The panic will cause the total failure of the cache if not recovered (this is not likely to happen because the http server will persumably catch it). If the panicked goroutine recovers, there's still goroutine leak (and also memory leak of not deleted call structs in the map) because its peer calls are waiting for the wait group. This behavior is verified in `singleflight/singleflight_wo_recover/sfwor.go`.

```go
// groupcache's implementation
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}
```

My implementation of singleflight handles panic on its own and gives all calls an error to indicate the panic happens and is handled. It's tested with a case in which a call deliberately panics after a short period of time and many goroutines make the call concurrently.

```go
// my implementation
func (g *Group) Do(key string, fn func() (interface{}, error)) (ret interface{}, retErr error) {
	g.mu.Lock()

	if g.m == nil {
		g.m = make(map[string]*call)
	}

	c, ok := g.m[key]

	// the call is duplicated
	if ok {
		g.mu.Unlock() // release lock
		log.Printf("[singleflight.Do] Blocked dup %s", key)
		c.wg.Wait()
		return c.ret, c.err
	}

	// this is a new call
	log.Printf("[singleflight.Do] Creating new call %s", key)
	c = new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Println("[singleflight.Do] Recovered from panic:", r)
			c.err = fmt.Errorf("call panicked and recovered in single flight: %s", r)
			c.wg.Done() // give clearance to all other Do()s waiting on this
			retErr = c.err
		}
		g.Forget(key) // this call removes the call from the map
		log.Printf("[singleflight.Do] Cleaning up call %s", key)
	}()

	log.Printf("[singleflight.Do] Doing new call %s", key)
	c.ret, c.err = fn()
	c.wg.Done()

	return c.ret, c.err
}
```


### Delete of virtual nodes in consistent hash

> An $O(n)$ implementation is given by [man-fish](https://github.com/man-fish) in the comments below the day4 tutorial.

Both geecache and groupcache don't support delete of nodes. Moreover, they store the hashed key that represent virtual nodes in an array of `uint32`. The array gives rise to the complexity of $O(log\ n)$ in time to find the key to be deleted and $O(n)$ in time to remove that. $O(n)$ is okay but we can do better with balanced binary search trees. I used the [BTree](https://pkg.go.dev/github.com/google/btree) implementation by google to **achieve delete in $O(log\ n)$ time** . Thanks, Google.

Btw, this support of delete/add nodes in real time is provided at upper layers like HTTPPool. The support at HTTPPool enables instructions of adding/removing nodes in the form of protobuf to be sent/received.

### A new problem (and its solution) with deleting nodes

> For better or worse, this is original. I realize the problem when implementing the above delete function.

**Background**

Hash collisio wasn't too much a problem in creating virtual nodes. It becomes a problem in my implementation because support of deleting nodes requires precise tracking of virtual nodes. The OG [implementation](https://github.com/golang/groupcache/blob/master/consistenthash/consistenthash.go) of consistent hash by grouphash shown below doesn't care about collision, which is perfectly fine. 

```go
func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.replicas; i++ {
			hash := int(m.hash([]byte(strconv.Itoa(i) + key)))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key
		}
	}
	sort.Ints(m.keys)
}
```

Even one virtual node of many virtual nodes under the name of physical node X is hashed to an existing key that has been already allocated to another virtual node that belongs to some other physical node Y. The former physical node X still have $(replicas - 1)$ virtual nodes on the ring hash. 

**Problem**

If we begin to delete nodes, such collision will lead to inconsistency and undermined performance. For example, physical node $A$ and $B$ are hashed to $[1,2]$ and $[2,3]$ respectively. Assume hashing of $B$ happens after that of $A$, and is deleted later. We will have node $A$ and its physical node $[1]$ remaining, but $A$ should also have $2$ as its virtual delegate. This undermines the performance of the hash as we could have one more node for physical node A.

**Solution**

The solution I proposed is very primitive and brutal. We append a random salt value of one byte to the name being hashed and store it (later I found that randomness is a bad idea). If there's still a collision, another salt is rolled. This process is allowed to try 10 salts before it gives up. I did some coarse calculation. Assuming that a million virtual nodes have been spawned, the replica factor is five and the possibility of hashing to any value is equal, the possibility of having at least one collision in a spawning five new replicas can be roughly approximated (the probability is "picking while not putting back") as follows.  
$\text{P} = 1-\text{P(No collision)} = 1-(\frac{2^{32}-1,000,000}{2^{32}})^5=0.9988$  
We are rolling this ten times in a row so this shouldn't cause any problem.

**Limitation**

Rolling the dice is a bad behavior given that we can just try from 0 to 255 in a ordered manner. I was kind of confused by the idea of "salt" and chose to generate it with randomly. Luckily this can be corrected, the maximum allowed retries for a new salt value can be configured and the salt comes from a function, which can also be easily replaced by a for loop.

## My Q and My A

Q: We have groups and we have consistent hash which sees keys only. Are keys from all groups distributed evenly among peers, or keys are distributed evenly *within* the group among peers.  
A: It could do both. Every group has its own peer picker field and the peer picker interface is implemented by HTTPPool, which owns the consistent hash. Depending on the need, we can do both. If we want the former, we register the same HTTPPool to all groups. If we want the latter, each group has its own HTTPPool that is responsible for sending queries and one global HTTPPool is used to handle queries received.  


## Lessons Learnt
1. It is helpful for testing to call a callback function when a black box system is conducting a operation that happens occasionally (like evicting cache entires).
2. Unit tests back refactoring and serve as regression tests.
3. Don't fiddle with coroutines when testing, especially when testing something that also involves concurrency. More explanation: If a query missed and shall consult the callback function to load data into cache, we can but should not write to a channel in the callback function and try to read (or just let the write block till time out and raise an error) the channel in the test logic to determine whether this miss should happen. This is just pointless. Instead of feeding a token to a channel to enable a select case in a callback function, why not just let the callback function add to a counter and check its value.  
4. `var _ interfaceT = (* concreteT)(nil)` validates if a concrete type implements an interface by converting a `nil pointer` to `pointer to interfaceT`.
