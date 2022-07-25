# better-groupcache

## What is this

This is my implementation of [geecache](https://geektutu.com/post/geecache.html), a educational tutorial that implements [groupcache](https://github.com/golang/groupcache). My implementation includes improvements on [geecache](https://geektutu.com/post/geecache.html) and even [groupcache](https://github.com/golang/groupcache). I am inventing the wheel again to practice go programming. 

## Major Improvements

### LRU-2

> This is suggested in the comments below the day1 tutorial by [liyu4](https://github.com/liyu4). 

The underlying cache method used by the two previous cache projects is **LRU**. A problem with LRU is that sporadic read/write could evict frequently accessed data because only one access is needed to load the data into cache (and evicts other data). **LRU-K** , which require K accesses to accommodate the data into cache, is a solution to the problem. I wrapped the original LRU with an FIFO queue that **implements LRU-2** while keeping the interfaces the same.

### More robust singleflight

Both geecache and groupcache implements *singleflight* that holds all but one calls to a function with the same key to uniquely identify these calls. After the one allowed call returns, the held calls are all released and share the same return values with the allowed one.

The problem is that, what if the function call panics. As shown below, if the call panics, the waiting group of this call will never done. The panic will cause the total failure of the cache. If the goroutine catches it, there's still goroutine leak (and also memory leak of not deleted calls in the map) because its peer calls are waiting for the wait group. This behavior is verified in `singleflight/singleflight_wo_recover/sfwor.go`.

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

### A new problem (and its solution) with deleting nodes

> For better or worse, this is finally original. I realize the problem when implementing the above delete function.

**Background**

Hash collision becomes a problem in our implementation that comes with delete support. The OG [implementation](https://github.com/golang/groupcache/blob/master/consistenthash/consistenthash.go) of consistent hash by grouphash shown below doesn't care about collision, which is perfectly fine. 

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

Even one virtual node is hashed to an existing key that represents another virtual node that belongs to some other physical node, we still have $(replicas - 1)$ nodes for that unfortune and deprived node. 

**Problem**

If we begin to delete nodes, such collision will lead to inconsistency and undermined performance. For example, physical node $A$ and $B$ are hashed to $[1,2]$ and $[2,3]$ respectively. Assume hashing of $B$ happens after that of $A$, and is deleted later. We will have node $A$ and its physical node $[1]$ remaining, but $A$ should also have $2$ as its virtual delegate. This undermines the performance of the hash as we could have one more node for physical node A.

**Solution**

The solution I proposed is very primitive and brutal. We append a random salt value of one byte to the name being hashed and store it (later I found that randomness is a bad idea). If there's still a collision, another salt is rolled. This process is allowed to try 10 salts before it gives up. I did some coarse calculation. Assuming that a million virtual nodes have been spawned, the replica factor is five and the possibility of hashing to any value is equal, the possibility of having at least one collision in a spawning five new replicas can be roughly approximated (the probability is "picking while not putting back") as follows.  
$\text{P} = 1-\text{P(No collision)} = 1-(\frac{2^{32}-1,000,000}{2^{32}})^5=0.9988$  
We are rolling this ten times in a row so this shouldn't cause any problem.

**Limitation**

Roll the dice is a bad behavior given that we can just try from 0 to 255 in a ordered manner. I was kind of confused by the idea of "salt" and chose to generate it with randomly. Luckily this can be corrected, the maximum allowed retries for a new salt value can be configured and the salt comes from a function, which can also be easily replaced by a for loop.

## My Q and My A

Q: We have groups and we have consistent hash which sees keys only. Are keys from all groups distributed evenly among peers, or keys are distributed evenly *within* the group among peers.  
A: It could do both. Every group has its own peer picker field and the peer picker interface is implemented by HTTPPool, which owns the consistent hash. Depending on the need, we can do both. If we want the former, we register the same HTTPPool to all groups. If we want the latter, each group has its own HTTPPool that is responsible for sending queries and one global HTTPPool is used to handle queries received.  

Q: In groupcache but not geecache, a comment in the function that gets keys from peers mentioned a TBD that they should use a QPS based approach to determine whether a fetched-from-remote keys should be deemed as hot and cached locally to reduce network traffic. I have a good idea to implement this.  
A: Indeed we can do it like the implementation of rate limiter of some API. I have a primitive idea that, we can have a map of keys to structs. The struct contains the key, a counter and last query time of it. We have a goroutine that scans it from time to time and remove outdated ones from the map. If a key is fetched frequently and the counter exceeds a limit, we can cache it. However, the problem is that, this design allocate extra space that shall be removed later. Flood queries may be excerbated with such extra moves taken and extra space allocateed. Just give it to rand may be just fine.

## Lessons Learnt
1. It is helpful for testing to call a callback function when a black box system is conducting a operation that happens occasionally (like evicting cache entires).
2. Unit tests back refactoring and serve as regression tests.
3. Don't fiddle with coroutines when testing, especially when testing something that also involves concurrency.
	For example, instead of feeding a token to a channel to enable a select case in a callback function, why not just let the callback function add to a counter and check its value.
4. `var _ interfaceT = (* concreteT)(nil)` validates if a concrete type implements an interface by converting a `nil pointer` to `pointer to interfaceT`.
