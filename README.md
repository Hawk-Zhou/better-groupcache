# better-groupcache

## What is this

This is my implementation of [geecache](https://geektutu.com/post/geecache.html), a educational tutorial that implements [groupcache](https://github.com/golang/groupcache). My implementation includes improvements on [geecache](https://geektutu.com/post/geecache.html) and even [groupcache](https://github.com/golang/groupcache). I am inventing the wheel again to practice go programming. 

## Major Improvements

### LRU-2

> This is suggested in the comments below the day1 tutorial by [liyu4](https://github.com/liyu4). 

The underlying cache method used by the two previous cache projects is **LRU**. A problem with LRU is that sporadic read/write could evict frequently accessed data because only one access is needed to load the data into cache (and evicts other data). **LRU-K** , which require K accesses to accommodate the data into cache, is a solution to the problem. I wrapped the original LRU with an FIFO queue that **implements LRU-2** while keeping the interfaces the same.

### Delete of virtual nodes in consistent hash

> An $O(n)$ implementation is given by [man-fish](https://github.com/man-fish) in the comments below the day4 tutorial.

Both geecache and groupcache don't support delete of nodes. Moreover, they store the hashed key that represent virtual nodes in an array of `uint32`. The array gives rise to the complexity of $O(log\ n)$ in time to find the key to be deleted and $O(n)$ in time to remove that. $O(n)$ is okay but we can do better with balanced binary search trees. I used the [BTree](https://pkg.go.dev/github.com/google/btree) implementation by google to **achieve delete in $O(log\ n)$ time** . Thanks, Google.

### A new problem (and its solution) with deleting nodes

> For better or worse, this is finally original. I realize this when implementing the above delete function.

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

The solution I proposed is very primitive and brutal. We append a random salt value of one byte to the name being hashed and store it (later I realized that this is a bad idea). If there's still a collision, another salt is rolled. This process is allowed to try 10 salts before it gives up. I did some bad calculation. Assuming that a million nodes have been replicated at a factor of five and the possibility of hashing to any value is equal, the possibility of having at least one collision in the five replicas can be calculated as follows.  
$\text{P} = 1-\text{P(No collision)} = 1-(\frac{2^{32}-1,000,000}{2^{32}})^5=0.9988$  
We are rolling this ten times in a row so this shouldn't lead to any problem.

**Limitation**

Roll the dice is a bad behavior given that we can just try from 0 to 255 in a ordered manner. I was kind of confused by the idea of "salt" and chose to generate it with randomly. Luckily this can be corrected, the maximum allowed retries for a new salt value can be configured and the salt comes from a function, which can also be easily replaced by a for loop.

## My Q and My A

Q: We have groups and we have consistent hash which sees keys only. Are keys from all groups distributed evenly among peers, or keys are distributed evenly *within* the group among peers.  
A: It could do both. Every group has its own peer picker field and the peer picker interface is implemented by HTTPPool, which owns the consistent hash. Depending on the need, we can do both. If we want the former, we register the same HTTPPool to all groups. If we want the latter, each group has its own HTTPPool that is responsible for sending queries and one global HTTPPool is used to handle queries received. 

## Lessons Learnt
1. It is helpful for testing to call a callback function when a black box system is conducting a operation that happens occasionally (like evicting cache entires).
2. Unit tests back refactoring and serve as regression tests.
3. Don't fiddle with coroutines when testing, especially when testing something that also involves concurrency.
	For example, instead of feeding a token to a channel to enable a select case in a callback function, why not just let the callback function add to a counter and check its value.
4. `var _ interfaceT = (* concreteT)(nil)` validates if a concrete type implements an interface by converting a `nil pointer` to `pointer to interfaceT`.
