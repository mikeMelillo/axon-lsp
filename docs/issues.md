Issue #1

In 'diff', the '{air}.set("return", marker())' throws up Argument changes expects Dict; received Obj?
This occurs in both places

A generic Obj? should be ambiguous enough that we do not flag it as an input type error

```
() => do
        readAll(temp and sensor and not rack and equipRef->crac and not discharge and not "return").each(s=> do
            if(s->navName.contains("Return")) diff(s, {air}.set("return", marker())).commit
            else if(s->navName.contains("Supply")) diff(s, {air}.set("discharge", marker())).commit
        end)
    end
```

Issue #2

'newRack' when used within the 'cabinets.each()' loop comes back as undefined even though it is defined above

Possibly mixing scope definitions, possibly not carrying larger scope into smaller scope

```
hubs.each(hub=> racks.each(r=> do
            newRackname: "Rack " + r
            aisle: rackIndexToAisle(r)
            rackItem: baseRack.merge({
                    navName: newRackname,
                    rackIndex: r,
                    spaceRef: hub->id,
                    networkRef: hub->networkRef,
                    aisle: aisle
                })
            existingRack: read(rackSpace and spaceRef == hub->id and networkRef == hub->networkRef and navName == newRackname, false)
            newRack:
                if(existingRack.isNull) diff(null, rackItem, {add}).commit
                else diff(existingRack, rackItem).commit
            cabinets.each(c=> do
                newCabinetName: "Cabinet " + c
                rackName: newRack->navName
                cabinetItem: baseCabinet.merge({
                    navName: newCabinetName,
                    cabinetIndex: c,
                    spaceRef: newRack->id,
                    networkRef: hub->networkRef,
                })

                existingCabinet: read(cabinet and spaceRef == newRack->id and networkRef == hub->networkRef and navName == newCabinetName, false)
                newCabinet:
                    if(existingCabinet.isNull) diff(null, cabinetItem, {add}).commit
                    else diff(existingCabinet, cabinetItem).commit
            end)
        end))
    ```


Issue #3 (possible feature add?)

The syntax below has an invalid 'if' filter. We should warn on bad syntax. Technically this pod compiles but function will fail to execute (since it's intepreted at runtime)

```
peerFanSpeed = read(point and his and fan and speed and equipRef==peer->id,false)
            if( or peerFanSpeed.isNull) do 
                out = {}.toGrid
                return null
            end
```


Issue #4

Below 'phase' within the diff dictionaries comes up as undefined variable. This is incorrect as phase is declaring an item in the dictionary, not a variable itself.

Possibly an issue with the dictionary syntax, e.g. {current ...} wants some value to show up, but it's just a marker.

```    () => do
        currentPoints: readAll(point and his and unit == "A")

        phA: currentPoints.findAll(x=> x["phaseA"].isNonNull)
        phA.each(r=> diff(r, {current, phase:"A"} ).commit )

        phB: currentPoints.findAll(x=> x["phaseB"].isNonNull)
        phB.each(r=> diff(r, {current, phase:"B"} ).commit )
        
        phC: currentPoints.findAll(x=> x["phaseC"].isNonNull)
        phC.each(r=> diff(r, {current, phase:"C"} ).commit )

        return {phA: phA.size, phB: phB.size, phC: phC.size}
        
    end```